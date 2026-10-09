package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/openai"
)

type assetLookupRequest struct {
	Hashes []string `json:"hashes"`
}

type assetLookupResponse struct {
	Assets []assetLookupRecord `json:"assets"`
}

type assetLookupRecord struct {
	SHA256   string `json:"sha256"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	NodeURL  string `json:"node_url,omitempty"`
	Origin   string `json:"origin,omitempty"`
	Master   bool   `json:"master,omitempty"`
}

func (assets *assetManager) handleNodeClusterAssetLookup(w http.ResponseWriter, r *http.Request) {
	if assets.identity().role != cluster.RoleMaster {
		openai.WriteError(w, http.StatusNotFound, "not_found", "cluster asset lookup is unavailable")
		return
	}
	defer r.Body.Close()
	var request assetLookupRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&request); err != nil || len(request.Hashes) == 0 || len(request.Hashes) > 256 {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid cluster asset lookup request")
		return
	}
	if hashes, ok := validatedLookupHashes(request.Hashes); ok {
		request.Hashes = hashes
	} else {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid cluster asset hash")
		return
	}
	openai.WriteJSON(w, http.StatusOK, assets.lookupClusterAssets(r.Context(), request))
}

type assetTransfer struct {
	done    chan struct{}
	success bool
}

type assetLookupCacheEntry struct {
	sources []assetLookupRecord
	expires time.Time
}

func (assets *assetManager) handleNodeAssetStream(w http.ResponseWriter, r *http.Request) {
	if assets.index == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "model asset was not found")
		return
	}
	hash := strings.TrimPrefix(r.URL.Path, "/router/v1/node/assets/")
	if !modelassets.ValidHash(hash) || strings.Contains(hash, "/") {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid asset hash")
		return
	}
	if !validAssetRange(r.Header.Get("Range")) {
		openai.WriteError(w, http.StatusRequestedRangeNotSatisfiable, "invalid_request_error", "invalid asset byte range")
		return
	}
	file, asset, err := assets.index.Open(hash)
	if err != nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "model asset was not found")
		return
	}
	defer file.Close()
	w.Header().Set(headerContentType, "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", asset.Filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, asset.Filename, asset.VerifiedAt, file)
}

func (assets *assetManager) resolvePeerAssetPath(hash string, filename string) (string, bool) {
	if assets.index == nil {
		return "", false
	}
	candidate := &assetTransfer{done: make(chan struct{})}
	value, loaded := assets.transfers.LoadOrStore(hash, candidate)
	transfer := value.(*assetTransfer)
	if loaded {
		<-transfer.done
		if !transfer.success {
			return "", false
		}
		return assets.index.Find(hash, filename)
	}
	defer func() {
		assets.transfers.Delete(hash)
		close(transfer.done)
	}()
	assets.transferSlots <- struct{}{}
	defer func() { <-assets.transferSlots }()
	for _, source := range assets.coordinatedAssetSources(hash) {
		if source.Filename != filename || source.NodeURL == "" || source.NodeURL == assets.identity().nodeURL {
			continue
		}
		if assets.pullKnownPeerAsset(source, hash, filename) {
			transfer.success = true
			if path, found := assets.index.Find(hash, filename); found {
				return path, true
			}
		}
	}
	return "", false
}

func (assets *assetManager) assetPeerURLs() []string {
	identity := assets.identity()
	if identity.registry == nil {
		return nil
	}
	values := make([]string, 0)
	for _, nodeURL := range identity.registry.NodeURLs() {
		if nodeURL != "" && nodeURL != identity.nodeURL {
			values = append(values, nodeURL)
		}
	}
	return values
}

func (assets *assetManager) pullKnownPeerAsset(source assetLookupRecord, hash string, filename string) bool {
	if source.SHA256 != hash || source.Filename != filename || source.Size < 0 || source.Size > assets.deps.maxTransferBytes() || source.NodeURL == "" {
		return false
	}
	partialPath := assets.peerPartialPath(hash)
	offset := partialAssetSize(partialPath, source.Size)
	transferContext, cancelTransfer := context.WithTimeout(context.Background(), assets.transferTimeout)
	defer cancelTransfer()
	response, err := assets.identity().client.StreamRange(transferContext, source.NodeURL, "/router/v1/node/assets/"+hash, offset)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return false
	}
	if offset > 0 && response.StatusCode == http.StatusPartialContent && !validContentRange(response.Header.Get("Content-Range"), offset, source.Size) {
		return false
	}
	return assets.promotePeerAsset(response.Body, hash, filename, source.Size, offset, response.StatusCode == http.StatusPartialContent)
}

func (assets *assetManager) coordinatedAssetSources(hash string) []assetLookupRecord {
	now := time.Now()
	assets.lookupMu.Lock()
	if cached, found := assets.lookupCache[hash]; found && now.Before(cached.expires) {
		sources := append([]assetLookupRecord{}, cached.sources...)
		assets.lookupMu.Unlock()
		return sources
	}
	assets.lookupMu.Unlock()
	sources := assets.lookupCoordinatedAssetSources(hash)
	assets.lookupMu.Lock()
	for key, cached := range assets.lookupCache {
		if now.After(cached.expires) {
			delete(assets.lookupCache, key)
		}
	}
	for len(assets.lookupCache) >= 512 {
		for key := range assets.lookupCache {
			delete(assets.lookupCache, key)
			break
		}
	}
	assets.lookupCache[hash] = assetLookupCacheEntry{sources: append([]assetLookupRecord{}, sources...), expires: now.Add(5 * time.Second)}
	assets.lookupMu.Unlock()
	return sources
}

func (assets *assetManager) masterAssetSources(records []assetLookupRecord) []assetLookupRecord {
	sources := make([]assetLookupRecord, 0, len(records))
	for _, record := range records {
		masterOwned := record.Master || record.NodeURL == ""
		record.Master = false
		if masterOwned {
			record.NodeURL = assets.identity().masterURL
		}
		sources = append(sources, record)
	}
	return sources
}

func (assets *assetManager) lookupCoordinatedAssetSources(hash string) []assetLookupRecord {
	identity := assets.identity()
	lookupContext, cancelLookup := context.WithTimeout(context.Background(), assets.lookupTimeout)
	defer cancelLookup()
	request := assetLookupRequest{Hashes: []string{hash}}
	if identity.role == cluster.RoleSlave && identity.masterURL != "" {
		var response assetLookupResponse
		if err := identity.client.JSON(lookupContext, http.MethodPost, identity.masterURL, "/router/v1/node/assets/lookup-cluster", request, &response); err == nil {
			return assets.masterAssetSources(response.Assets)
		}
	}
	if identity.role == cluster.RoleMaster {
		return assets.lookupClusterAssets(lookupContext, request).Assets
	}
	values := make([]assetLookupRecord, 0)
	for _, nodeURL := range assets.assetPeerURLs() {
		var response assetLookupResponse
		if err := identity.client.JSON(lookupContext, http.MethodPost, nodeURL, "/router/v1/node/assets/lookup", request, &response); err != nil {
			continue
		}
		for _, asset := range response.Assets {
			asset.NodeURL = nodeURL
			values = append(values, asset)
		}
	}
	return values
}

func (assets *assetManager) peerPartialPath(hash string) string {
	return filepath.Join(assets.index.SharedDir(), "sha256", hash[:2], hash, ".partial")
}

func partialAssetSize(path string, expectedSize int64) int64 {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() >= expectedSize {
		return 0
	}
	return info.Size()
}

func validContentRange(value string, offset int64, size int64) bool {
	expectedPrefix := "bytes " + strconv.FormatInt(offset, 10) + "-"
	return strings.HasPrefix(value, expectedPrefix) && strings.HasSuffix(value, "/"+strconv.FormatInt(size, 10))
}

func validAssetRange(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 96 || !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if len(parts) != 2 || parts[0] == "" && parts[1] == "" {
		return false
	}
	for _, part := range parts {
		if part == "" {
			continue
		}
		if _, err := strconv.ParseUint(part, 10, 63); err != nil {
			return false
		}
	}
	return true
}

func validatedLookupHashes(values []string) ([]string, bool) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, hash := range values {
		if !modelassets.ValidHash(hash) {
			return nil, false
		}
		if _, found := seen[hash]; found {
			continue
		}
		seen[hash] = struct{}{}
		result = append(result, hash)
	}
	return result, true
}

func secureAssetDirectory(root string, target string) bool {
	root, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	current, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	if info, err := os.Lstat(current); err != nil || !info.IsDir() {
		return false
	}
	if relative == "." {
		return true
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

const maxAssetLookupHashes = 256

func (assets *assetManager) lookupClusterAssets(ctx context.Context, request assetLookupRequest) assetLookupResponse {
	identity := assets.identity()
	response := assetLookupResponse{Assets: []assetLookupRecord{}}
	if assets.index != nil {
		for _, hash := range request.Hashes {
			if record, found := assets.localAssetRecord(assetLookupRecord{SHA256: hash, NodeURL: identity.nodeURL, Master: true}); found {
				response.Assets = append(response.Assets, record)
			}
		}
	}
	response.Assets = append(response.Assets, peerAssetRecords(ctx, identity, assets.assetPeerURLs(), request)...)
	return response
}

func (assets *assetManager) localAssetRecord(record assetLookupRecord) (assetLookupRecord, bool) {
	found := false
	if asset, assetFound := assets.index.Lookup(record.SHA256); assetFound {
		record.Filename, record.Size = asset.Filename, asset.Size
		found = true
	}
	if origin, originFound := assets.index.Origin(record.SHA256); originFound {
		record.Origin = origin.URI()
		found = true
	}
	return record, found
}

func peerAssetRecords(ctx context.Context, identity clusterIdentity, urls []string, request assetLookupRequest) []assetLookupRecord {
	results := make(chan assetLookupResponse, len(urls))
	var group sync.WaitGroup
	for _, nodeURL := range urls {
		group.Go(func() {
			var peer assetLookupResponse
			if err := identity.client.JSON(ctx, http.MethodPost, nodeURL, "/router/v1/node/assets/lookup", request, &peer); err != nil {
				return
			}
			for index := range peer.Assets {
				peer.Assets[index].NodeURL = nodeURL
			}
			results <- peer
		})
	}
	group.Wait()
	close(results)
	var records []assetLookupRecord
	for peer := range results {
		records = append(records, peer.Assets...)
	}
	return records
}

func (assets *assetManager) handleNodeAssetLookup(w http.ResponseWriter, r *http.Request) {
	if assets.index == nil {
		openai.WriteJSON(w, http.StatusOK, assetLookupResponse{Assets: []assetLookupRecord{}})
		return
	}
	defer r.Body.Close()
	var request assetLookupRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&request); err != nil || len(request.Hashes) > maxAssetLookupHashes {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid asset lookup request")
		return
	}
	hashes, ok := validatedLookupHashes(request.Hashes)
	if !ok {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid asset hash")
		return
	}
	response := assetLookupResponse{Assets: make([]assetLookupRecord, 0, len(hashes))}
	for _, hash := range hashes {
		if record, found := assets.localAssetRecord(assetLookupRecord{SHA256: hash}); found {
			response.Assets = append(response.Assets, record)
		}
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

type peerAssetTransfer struct {
	source  io.Reader
	hash    string
	size    int64
	offset  int64
	partial bool
}

func (assets *assetManager) promotePeerAsset(source io.Reader, expectedHash string, filename string, size int64, offset int64, partialResponse bool) bool {
	if !modelassets.ValidHash(expectedHash) || !modelassets.SafeFilename(filename) || size < 0 || size > assets.deps.maxTransferBytes() {
		return false
	}
	targetDir := filepath.Join(assets.index.SharedDir(), "sha256", expectedHash[:2], expectedHash)
	if err := os.MkdirAll(targetDir, 0o755); err != nil || !secureAssetDirectory(assets.index.SharedDir(), targetDir) {
		return false
	}
	temporaryPath := assets.peerPartialPath(expectedHash)
	temporary, ok := openRegularPartialFile(temporaryPath)
	if !ok {
		return false
	}
	transfer := peerAssetTransfer{source: source, hash: expectedHash, size: size, offset: offset, partial: partialResponse}
	if !transfer.writeVerified(temporary, temporaryPath) {
		return false
	}
	return assets.installPeerAsset(temporaryPath, filepath.Join(targetDir, filename), expectedHash)
}

func openRegularPartialFile(path string) (*os.File, bool) {
	if !regularFileOrAbsent(path) {
		return nil, false
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|openNoFollow, 0o600)
	if err != nil {
		return nil, false
	}
	openedInfo, openedErr := file.Stat()
	pathInfo, pathErr := os.Lstat(path)
	if openedErr != nil || pathErr != nil || !openedInfo.Mode().IsRegular() || !pathInfo.Mode().IsRegular() || !os.SameFile(openedInfo, pathInfo) {
		file.Close()
		return nil, false
	}
	return file, true
}

func regularFileOrAbsent(path string) bool {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return true
	}
	return err == nil && info.Mode().IsRegular()
}

func (transfer peerAssetTransfer) writeVerified(temporary *os.File, temporaryPath string) bool {
	digest := sha256.New()
	offset, ok := transfer.resumeOffset(temporary, digest)
	if !ok {
		temporary.Close()
		return false
	}
	if _, err := temporary.Seek(offset, io.SeekStart); err != nil {
		temporary.Close()
		return false
	}
	written, copyErr := io.Copy(io.MultiWriter(temporary, digest), io.LimitReader(transfer.source, transfer.size-offset+1))
	if copyErr != nil || written != transfer.size-offset {
		temporary.Close()
		return false
	}
	if hex.EncodeToString(digest.Sum(nil)) != transfer.hash {
		temporary.Close()
		_ = os.Remove(temporaryPath)
		return false
	}
	return temporary.Sync() == nil && temporary.Close() == nil
}

func (transfer peerAssetTransfer) resumeOffset(temporary *os.File, digest io.Writer) (int64, bool) {
	if !transfer.partial {
		return 0, temporary.Truncate(0) == nil
	}
	if transfer.offset <= 0 {
		return transfer.offset, true
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return 0, false
	}
	hashed, err := io.CopyN(digest, temporary, transfer.offset)
	return transfer.offset, err == nil && hashed == transfer.offset
}

func (assets *assetManager) installPeerAsset(temporaryPath string, target string, expectedHash string) bool {
	if _, err := os.Lstat(target); err == nil {
		_ = os.Remove(temporaryPath)
		asset, indexErr := assets.index.IndexFile(target)
		return indexErr == nil && asset.SHA256 == expectedHash && assets.index.SetVerificationSource(expectedHash, "peer_sha256") == nil
	} else if !os.IsNotExist(err) {
		return false
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return false
	}
	if _, err := assets.index.IndexFile(target); err != nil {
		return false
	}
	return assets.index.SetVerificationSource(expectedHash, "peer_sha256") == nil
}
