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
}

func (service *Service) handleNodeClusterAssetLookup(w http.ResponseWriter, r *http.Request) {
	if service.clusterRole != cluster.RoleMaster {
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
	openai.WriteJSON(w, http.StatusOK, service.lookupClusterAssets(r.Context(), request))
}

func (service *Service) lookupClusterAssets(ctx context.Context, request assetLookupRequest) assetLookupResponse {
	response := assetLookupResponse{Assets: []assetLookupRecord{}}
	if service.assetIndex != nil {
		for _, hash := range request.Hashes {
			record := assetLookupRecord{SHA256: hash, NodeURL: service.nodeURL}
			found := false
			if asset, assetFound := service.assetIndex.Lookup(hash); assetFound {
				record.Filename, record.Size = asset.Filename, asset.Size
				found = true
			}
			if origin, originFound := service.assetIndex.Origin(hash); originFound {
				record.Origin = origin.URI()
				found = true
			}
			if found {
				response.Assets = append(response.Assets, record)
			}
		}
	}
	urls := service.assetPeerURLs()
	results := make(chan assetLookupResponse, len(urls))
	var group sync.WaitGroup
	for _, nodeURL := range urls {
		group.Add(1)
		go func(nodeURL string) {
			defer group.Done()
			var peer assetLookupResponse
			if err := service.clusterClient.JSON(ctx, http.MethodPost, nodeURL, "/router/v1/node/assets/lookup", request, &peer); err != nil {
				return
			}
			for index := range peer.Assets {
				peer.Assets[index].NodeURL = nodeURL
			}
			results <- peer
		}(nodeURL)
	}
	group.Wait()
	close(results)
	for peer := range results {
		response.Assets = append(response.Assets, peer.Assets...)
	}
	return response
}

type assetTransfer struct {
	done    chan struct{}
	success bool
}

type assetLookupCacheEntry struct {
	sources []assetLookupRecord
	expires time.Time
}

func (service *Service) handleNodeAssetLookup(w http.ResponseWriter, r *http.Request) {
	if service.assetIndex == nil {
		openai.WriteJSON(w, http.StatusOK, assetLookupResponse{Assets: []assetLookupRecord{}})
		return
	}
	defer r.Body.Close()
	var request assetLookupRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&request); err != nil || len(request.Hashes) > 256 {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid asset lookup request")
		return
	}
	if hashes, ok := validatedLookupHashes(request.Hashes); ok {
		request.Hashes = hashes
	} else {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid asset hash")
		return
	}
	response := assetLookupResponse{Assets: make([]assetLookupRecord, 0, len(request.Hashes))}
	for _, hash := range request.Hashes {
		record := assetLookupRecord{SHA256: hash}
		found := false
		if asset, assetFound := service.assetIndex.Lookup(hash); assetFound {
			record.Filename, record.Size = asset.Filename, asset.Size
			found = true
		}
		if origin, originFound := service.assetIndex.Origin(hash); originFound {
			record.Origin = origin.URI()
			found = true
		}
		if found {
			response.Assets = append(response.Assets, record)
		}
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleNodeAssetStream(w http.ResponseWriter, r *http.Request) {
	if service.assetIndex == nil {
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
	file, asset, err := service.assetIndex.Open(hash)
	if err != nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "model asset was not found")
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", asset.Filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, asset.Filename, asset.VerifiedAt, file)
}

func (service *Service) resolvePeerAssetPath(hash string, filename string) (string, bool) {
	if service.assetIndex == nil {
		return "", false
	}
	candidate := &assetTransfer{done: make(chan struct{})}
	value, loaded := service.assetTransfers.LoadOrStore(hash, candidate)
	transfer := value.(*assetTransfer)
	if loaded {
		<-transfer.done
		if !transfer.success {
			return "", false
		}
		return service.assetIndex.Find(hash, filename)
	}
	defer func() {
		service.assetTransfers.Delete(hash)
		close(transfer.done)
	}()
	service.assetTransferSlots <- struct{}{}
	defer func() { <-service.assetTransferSlots }()
	for _, source := range service.coordinatedAssetSources(hash) {
		if source.Filename != filename || source.NodeURL == "" || source.NodeURL == service.nodeURL {
			continue
		}
		if service.pullKnownPeerAsset(source, hash, filename) {
			transfer.success = true
			if path, found := service.assetIndex.Find(hash, filename); found {
				return path, true
			}
		}
	}
	return "", false
}

func (service *Service) assetPeerURLs() []string {
	if service.registry == nil {
		return nil
	}
	values := make([]string, 0)
	for _, nodeURL := range service.registry.NodeURLs() {
		if nodeURL != "" && nodeURL != service.nodeURL {
			values = append(values, nodeURL)
		}
	}
	return values
}

func (service *Service) pullPeerAsset(nodeURL string, hash string, filename string) bool {
	if !modelassets.ValidHash(hash) || !modelassets.SafeFilename(filename) || service.assetIndex == nil {
		return false
	}
	var lookup assetLookupResponse
	if err := service.clusterClient.JSON(routedAssetContext(), http.MethodPost, nodeURL, "/router/v1/node/assets/lookup", assetLookupRequest{Hashes: []string{hash}}, &lookup); err != nil {
		return false
	}
	if len(lookup.Assets) != 1 || lookup.Assets[0].SHA256 != hash || lookup.Assets[0].Filename != filename || lookup.Assets[0].Size < 0 || lookup.Assets[0].Size > service.transportLimits.MaxResponseBytes {
		return false
	}
	lookup.Assets[0].NodeURL = nodeURL
	return service.pullKnownPeerAsset(lookup.Assets[0], hash, filename)
}

func (service *Service) pullKnownPeerAsset(source assetLookupRecord, hash string, filename string) bool {
	if source.SHA256 != hash || source.Filename != filename || source.Size < 0 || source.Size > service.transportLimits.MaxResponseBytes || source.NodeURL == "" {
		return false
	}
	partialPath := service.peerPartialPath(hash)
	offset := partialAssetSize(partialPath, source.Size)
	response, err := service.clusterClient.StreamRange(routedAssetContext(), source.NodeURL, "/router/v1/node/assets/"+hash, offset)
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
	return service.promotePeerAsset(response.Body, hash, filename, source.Size, offset, response.StatusCode == http.StatusPartialContent)
}

func (service *Service) coordinatedAssetSources(hash string) []assetLookupRecord {
	now := time.Now()
	service.assetLookupMu.Lock()
	if cached, found := service.assetLookupCache[hash]; found && now.Before(cached.expires) {
		sources := append([]assetLookupRecord{}, cached.sources...)
		service.assetLookupMu.Unlock()
		return sources
	}
	service.assetLookupMu.Unlock()
	sources := service.lookupCoordinatedAssetSources(hash)
	service.assetLookupMu.Lock()
	for key, cached := range service.assetLookupCache {
		if now.After(cached.expires) {
			delete(service.assetLookupCache, key)
		}
	}
	for len(service.assetLookupCache) >= 512 {
		for key := range service.assetLookupCache {
			delete(service.assetLookupCache, key)
			break
		}
	}
	service.assetLookupCache[hash] = assetLookupCacheEntry{sources: append([]assetLookupRecord{}, sources...), expires: now.Add(5 * time.Second)}
	service.assetLookupMu.Unlock()
	return sources
}

func (service *Service) lookupCoordinatedAssetSources(hash string) []assetLookupRecord {
	request := assetLookupRequest{Hashes: []string{hash}}
	if service.clusterRole == cluster.RoleSlave && service.masterURL != "" {
		var response assetLookupResponse
		if err := service.clusterClient.JSON(routedAssetContext(), http.MethodPost, service.masterURL, "/router/v1/node/assets/lookup-cluster", request, &response); err == nil {
			return response.Assets
		}
	}
	if service.clusterRole == cluster.RoleMaster {
		return service.lookupClusterAssets(routedAssetContext(), request).Assets
	}
	values := make([]assetLookupRecord, 0)
	for _, nodeURL := range service.assetPeerURLs() {
		var response assetLookupResponse
		if err := service.clusterClient.JSON(routedAssetContext(), http.MethodPost, nodeURL, "/router/v1/node/assets/lookup", request, &response); err != nil {
			continue
		}
		for _, asset := range response.Assets {
			asset.NodeURL = nodeURL
			values = append(values, asset)
		}
	}
	return values
}

func routedAssetContext() context.Context { return context.Background() }

func (service *Service) promotePeerAsset(source io.Reader, expectedHash string, filename string, size int64, offset int64, partialResponse bool) bool {
	if !modelassets.ValidHash(expectedHash) || !modelassets.SafeFilename(filename) || size < 0 || size > service.transportLimits.MaxResponseBytes {
		return false
	}
	targetDir := filepath.Join(service.assetIndex.SharedDir(), "sha256", expectedHash[:2], expectedHash)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return false
	}
	if !secureAssetDirectory(service.assetIndex.SharedDir(), targetDir) {
		return false
	}
	temporaryPath := service.peerPartialPath(expectedHash)
	temporary, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false
	}
	openedInfo, openedErr := temporary.Stat()
	pathInfo, pathErr := os.Lstat(temporaryPath)
	if openedErr != nil || pathErr != nil || !openedInfo.Mode().IsRegular() || !pathInfo.Mode().IsRegular() || !os.SameFile(openedInfo, pathInfo) {
		temporary.Close()
		return false
	}
	digest := sha256.New()
	if !partialResponse {
		offset = 0
		if temporary.Truncate(0) != nil {
			temporary.Close()
			return false
		}
	}
	if offset > 0 {
		if _, err := temporary.Seek(0, io.SeekStart); err != nil {
			temporary.Close()
			return false
		}
		hashed, err := io.CopyN(digest, temporary, offset)
		if err != nil || hashed != offset {
			temporary.Close()
			return false
		}
	}
	if _, err := temporary.Seek(offset, io.SeekStart); err != nil {
		temporary.Close()
		return false
	}
	written, copyErr := io.Copy(io.MultiWriter(temporary, digest), io.LimitReader(source, size-offset+1))
	if copyErr != nil || written != size-offset {
		temporary.Close()
		return false
	}
	if hex.EncodeToString(digest.Sum(nil)) != expectedHash {
		temporary.Close()
		_ = os.Remove(temporaryPath)
		return false
	}
	if temporary.Sync() != nil || temporary.Close() != nil {
		return false
	}
	target := filepath.Join(targetDir, filename)
	if _, err := os.Lstat(target); err == nil {
		_ = os.Remove(temporaryPath)
		asset, indexErr := service.assetIndex.IndexFile(target)
		return indexErr == nil && asset.SHA256 == expectedHash && service.assetIndex.SetVerificationSource(expectedHash, "peer_sha256") == nil
	} else if !os.IsNotExist(err) {
		return false
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return false
	}
	_, err = service.assetIndex.IndexFile(target)
	if err != nil {
		return false
	}
	return service.assetIndex.SetVerificationSource(expectedHash, "peer_sha256") == nil
}

func (service *Service) peerPartialPath(hash string) string {
	return filepath.Join(service.assetIndex.SharedDir(), "sha256", hash[:2], hash, ".partial")
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
	current := root
	if info, err := os.Lstat(current); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
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
