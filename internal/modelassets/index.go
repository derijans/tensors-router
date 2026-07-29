package modelassets

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Asset struct {
	SHA256             string    `json:"sha256"`
	Filename           string    `json:"filename"`
	Size               int64     `json:"size"`
	Path               string    `json:"path"`
	Repository         string    `json:"repository,omitempty"`
	RepositoryPath     string    `json:"repository_path,omitempty"`
	Commit             string    `json:"commit,omitempty"`
	VerificationSource string    `json:"verification_source,omitempty"`
	VerifiedAt         time.Time `json:"verified_at"`
}

type cachedPath struct {
	Size        int64  `json:"size"`
	ModTimeNano int64  `json:"mod_time_nano"`
	SHA256      string `json:"sha256"`
}

type persistedIndex struct {
	Version int                   `json:"version"`
	Assets  map[string]Asset      `json:"assets"`
	Paths   map[string]cachedPath `json:"paths"`
}

type Index struct {
	mu          sync.Mutex
	db          *sql.DB
	sharedDir   string
	assets      map[string]Asset
	paths       map[string]cachedPath
	origins     map[string]Origin
	flights     map[string]*hashFlight
	hashWorkers int
}

type hashFlight struct {
	done chan struct{}
	hash string
	err  error
}

func NewIndex(storeDir string, sharedDir string) (*Index, error) {
	if strings.TrimSpace(storeDir) == "" {
		return nil, fmt.Errorf("asset store directory is required")
	}
	if strings.TrimSpace(sharedDir) == "" {
		sharedDir = filepath.Join(storeDir, "model-assets")
	}
	if err := os.MkdirAll(storeDir, 0o700); err != nil {
		return nil, err
	}
	databasePath := filepath.Join(storeDir, "model-assets.sqlite")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, err
	}
	index := &Index{db: db, sharedDir: filepath.Clean(sharedDir), assets: map[string]Asset{}, paths: map[string]cachedPath{}, origins: map[string]Origin{}, flights: map[string]*hashFlight{}, hashWorkers: 1}
	if err := index.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(databasePath, 0o600); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := index.load(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return index, nil
}

func (index *Index) initialize() error {
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS assets(sha256 TEXT PRIMARY KEY, filename TEXT NOT NULL, size INTEGER NOT NULL, path TEXT NOT NULL, repository TEXT NOT NULL DEFAULT '', repository_path TEXT NOT NULL DEFAULT '', commit_hash TEXT NOT NULL DEFAULT '', verification_source TEXT NOT NULL DEFAULT '', verified_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS path_cache(path TEXT PRIMARY KEY, size INTEGER NOT NULL, mod_time_nano INTEGER NOT NULL, sha256 TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS hf_origins(sha256 TEXT PRIMARY KEY, repository TEXT NOT NULL, repository_path TEXT NOT NULL, commit_hash TEXT NOT NULL, verified_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS resolution_jobs(id TEXT PRIMARY KEY, config_id TEXT NOT NULL, node_id TEXT NOT NULL, state TEXT NOT NULL, source TEXT NOT NULL DEFAULT '', progress_bytes INTEGER NOT NULL DEFAULT 0, total_bytes INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS resolution_results(job_id TEXT NOT NULL, field_name TEXT NOT NULL, sha256 TEXT NOT NULL, resolved INTEGER NOT NULL, source TEXT NOT NULL DEFAULT '', error TEXT NOT NULL DEFAULT '', PRIMARY KEY(job_id, field_name, sha256), FOREIGN KEY(job_id) REFERENCES resolution_jobs(id) ON DELETE CASCADE)`,
	}
	for _, statement := range statements {
		if _, err := index.db.Exec(statement); err != nil {
			return err
		}
	}
	_, _ = index.db.Exec(`ALTER TABLE assets ADD COLUMN verification_source TEXT NOT NULL DEFAULT ''`)
	_, _ = index.db.Exec(`ALTER TABLE resolution_results ADD COLUMN verification TEXT NOT NULL DEFAULT ''`)
	_, _ = index.db.Exec(`ALTER TABLE resolution_results ADD COLUMN commit_hash TEXT NOT NULL DEFAULT ''`)
	return nil
}

func (index *Index) load() error {
	rows, err := index.db.Query(`SELECT sha256, filename, size, path, repository, repository_path, commit_hash, verification_source, verified_at FROM assets`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var asset Asset
		var verified string
		if err := rows.Scan(&asset.SHA256, &asset.Filename, &asset.Size, &asset.Path, &asset.Repository, &asset.RepositoryPath, &asset.Commit, &asset.VerificationSource, &verified); err != nil {
			_ = rows.Close()
			return err
		}
		if !validHash(asset.SHA256) || !safeFilename(asset.Filename) || asset.Size < 0 {
			continue
		}
		asset.VerifiedAt, _ = time.Parse(time.RFC3339Nano, verified)
		index.assets[asset.SHA256] = asset
		origin := Origin{Repository: asset.Repository, Commit: asset.Commit, Path: asset.RepositoryPath}
		if origin.URI() != "" {
			index.origins[asset.SHA256] = origin
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	originRows, err := index.db.Query(`SELECT sha256, repository, repository_path, commit_hash FROM hf_origins`)
	if err != nil {
		return err
	}
	for originRows.Next() {
		var hash string
		var origin Origin
		if err := originRows.Scan(&hash, &origin.Repository, &origin.Path, &origin.Commit); err != nil {
			_ = originRows.Close()
			return err
		}
		index.origins[hash] = origin
	}
	if err := originRows.Close(); err != nil {
		return err
	}
	cacheRows, err := index.db.Query(`SELECT path, size, mod_time_nano, sha256 FROM path_cache`)
	if err != nil {
		return err
	}
	defer cacheRows.Close()
	for cacheRows.Next() {
		var path string
		var cached cachedPath
		if err := cacheRows.Scan(&path, &cached.Size, &cached.ModTimeNano, &cached.SHA256); err != nil {
			return err
		}
		index.paths[path] = cached
	}
	return cacheRows.Err()
}

func (index *Index) SharedDir() string { return index.sharedDir }

func (index *Index) SetHashWorkers(workers int) {
	if workers < 1 {
		workers = 1
	}
	index.hashWorkers = workers
}

func (index *Index) IndexFile(path string) (Asset, error) {
	asset, err := index.indexFile(path)
	if err != nil {
		return Asset{}, err
	}
	if err := index.persistAsset(asset); err != nil {
		return Asset{}, err
	}
	return asset, nil
}

func (index *Index) CachedFileHash(path string) (string, bool) {
	absolute, info, err := regularFile(path)
	if err != nil {
		return "", false
	}
	index.mu.Lock()
	cached, found := index.paths[absolute]
	index.mu.Unlock()
	if !found || cached.Size != info.Size() || cached.ModTimeNano != info.ModTime().UnixNano() || !validHash(cached.SHA256) {
		return "", false
	}
	return cached.SHA256, true
}

func (index *Index) indexFile(path string) (Asset, error) {
	requestedPath, err := filepath.Abs(path)
	if err != nil {
		return Asset{}, err
	}
	filename := filepath.Base(requestedPath)
	if !safeFilename(filename) {
		return Asset{}, fmt.Errorf("asset has an unsafe filename")
	}
	absolute, info, err := regularFile(path)
	if err != nil {
		return Asset{}, err
	}
	hash, err := index.hashFile(absolute, info)
	if err != nil {
		return Asset{}, err
	}
	asset := Asset{SHA256: hash, Filename: filename, Size: info.Size(), Path: absolute, VerificationSource: "sha256", VerifiedAt: time.Now().UTC()}
	index.mu.Lock()
	index.assets[hash] = asset
	index.mu.Unlock()
	return asset, nil
}

func (index *Index) IndexRoots(roots []string) error {
	paths := make([]string, 0)
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() || !entry.Type().IsRegular() {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
		if err != nil {
			return err
		}
	}
	jobs := make(chan string)
	errors := make(chan error, index.hashWorkers)
	var workers sync.WaitGroup
	for worker := 0; worker < index.hashWorkers; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for path := range jobs {
				if _, err := index.indexFile(path); err != nil {
					select {
					case errors <- err:
					default:
					}
				}
			}
		}()
	}
	for _, path := range paths {
		jobs <- path
	}
	close(jobs)
	workers.Wait()
	close(errors)
	if err := <-errors; err != nil {
		return err
	}
	return index.Save()
}

func (index *Index) IndexConfigReferences(configDir string) error {
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(configDir, func(configPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(entry.Name()), ".kcpps") {
			return nil
		}
		file, err := os.Open(configPath)
		if err != nil {
			return nil
		}
		var config map[string]any
		decodeErr := json.NewDecoder(io.LimitReader(file, 8<<20)).Decode(&config)
		_ = file.Close()
		if decodeErr != nil {
			return nil
		}
		for field, value := range config {
			if !isModelField(field) {
				continue
			}
			paths, _, ok := pathValues(value)
			if !ok {
				continue
			}
			for _, modelPath := range paths {
				_, _ = index.IndexFile(modelPath)
			}
		}
		return nil
	})
}

func (index *Index) BindOrigin(hash string, origin Origin) error {
	if !validHash(hash) || origin.URI() == "" {
		return fmt.Errorf("invalid asset origin")
	}
	index.mu.Lock()
	index.origins[hash] = origin
	var boundAsset Asset
	assetFound := false
	if asset, found := index.assets[hash]; found {
		asset.Repository, asset.Commit, asset.RepositoryPath = origin.Repository, origin.Commit, origin.Path
		index.assets[hash] = asset
		boundAsset, assetFound = asset, true
	}
	index.mu.Unlock()
	if _, err := index.db.Exec(`INSERT INTO hf_origins(sha256, repository, repository_path, commit_hash, verified_at) VALUES(?, ?, ?, ?, ?) ON CONFLICT(sha256) DO UPDATE SET repository=excluded.repository, repository_path=excluded.repository_path, commit_hash=excluded.commit_hash, verified_at=excluded.verified_at`, hash, origin.Repository, origin.Path, origin.Commit, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if assetFound {
		return index.persistAsset(boundAsset)
	}
	return nil
}

func (index *Index) SetVerificationSource(hash string, source string) error {
	allowed := map[string]bool{"sha256": true, "peer_sha256": true, "hf_lfs_sha256": true, "download": true, "sidecar": true, "scan": true}
	if !validHash(hash) || !allowed[source] {
		return fmt.Errorf("invalid verification source")
	}
	index.mu.Lock()
	asset, found := index.assets[hash]
	if found {
		asset.VerificationSource = source
		asset.VerifiedAt = time.Now().UTC()
		index.assets[hash] = asset
	}
	index.mu.Unlock()
	if !found {
		return fmt.Errorf("asset was not found")
	}
	return index.persistAsset(asset)
}

func (index *Index) Origin(hash string) (Origin, bool) {
	index.mu.Lock()
	defer index.mu.Unlock()
	origin, found := index.origins[hash]
	if !found {
		return Origin{}, false
	}
	return origin, origin.URI() != ""
}

func (index *Index) Find(hash string, filename string) (string, bool) {
	if !validHash(hash) || !safeFilename(filename) {
		return "", false
	}
	index.mu.Lock()
	asset, found := index.assets[hash]
	index.mu.Unlock()
	if !found || asset.Filename != filename {
		return "", false
	}
	absolute, info, err := regularFile(asset.Path)
	if err != nil || info.Size() != asset.Size {
		return "", false
	}
	actualHash, err := index.hashFile(absolute, info)
	if err != nil || actualHash != hash {
		return "", false
	}
	return absolute, true
}

func (index *Index) FindInRoots(hash string, filename string, roots []string) (string, bool, error) {
	if path, found := index.Find(hash, filename); found {
		return path, true, nil
	}
	if !validHash(hash) || !safeFilename(filename) {
		return "", false, nil
	}
	candidates := make([]string, 0)
	seenRoots := map[string]struct{}{}
	for _, root := range append(append([]string{}, roots...), index.sharedDir) {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			return "", false, err
		}
		if _, seen := seenRoots[absoluteRoot]; seen {
			continue
		}
		seenRoots[absoluteRoot] = struct{}{}
		if _, err := os.Stat(absoluteRoot); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return "", false, err
		}
		if err := filepath.WalkDir(absoluteRoot, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if !entry.IsDir() && entry.Type().IsRegular() && entry.Name() == filename {
				candidates = append(candidates, path)
			}
			return nil
		}); err != nil {
			return "", false, err
		}
	}
	for _, candidate := range candidates {
		asset, err := index.IndexFile(candidate)
		if err != nil {
			return "", false, err
		}
		if asset.SHA256 == hash {
			return asset.Path, true, nil
		}
	}
	return "", false, nil
}

func (index *Index) Assets() []Asset {
	index.mu.Lock()
	defer index.mu.Unlock()
	assets := make([]Asset, 0, len(index.assets))
	for _, asset := range index.assets {
		assets = append(assets, asset)
	}
	sort.Slice(assets, func(left, right int) bool { return assets[left].SHA256 < assets[right].SHA256 })
	return assets
}

func (index *Index) Lookup(hash string) (Asset, bool) {
	if !validHash(hash) {
		return Asset{}, false
	}
	index.mu.Lock()
	asset, found := index.assets[hash]
	index.mu.Unlock()
	if !found || !safeFilename(asset.Filename) {
		return Asset{}, false
	}
	absolute, info, err := regularFile(asset.Path)
	if err != nil || info.Size() != asset.Size {
		return Asset{}, false
	}
	actualHash, err := index.hashFile(absolute, info)
	if err != nil || actualHash != hash {
		return Asset{}, false
	}
	asset.Path = absolute
	return asset, true
}

func (index *Index) Open(hash string) (*os.File, Asset, error) {
	asset, found := index.Lookup(hash)
	if !found {
		return nil, Asset{}, fmt.Errorf("asset was not found")
	}
	file, err := os.Open(asset.Path)
	if err != nil {
		return nil, Asset{}, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.Size {
		_ = file.Close()
		return nil, Asset{}, fmt.Errorf("asset changed while opening")
	}
	actualHash, err := hashOpenFile(file)
	if err != nil || actualHash != hash {
		_ = file.Close()
		return nil, Asset{}, fmt.Errorf("asset content failed revalidation")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, Asset{}, fmt.Errorf("asset could not be opened")
	}
	return file, asset, nil
}

func (index *Index) Save() error {
	index.mu.Lock()
	persisted := persistedIndex{Version: 1, Assets: cloneAssets(index.assets), Paths: clonePaths(index.paths)}
	index.mu.Unlock()
	transaction, err := index.db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	for _, asset := range persisted.Assets {
		if _, err := transaction.Exec(`INSERT INTO assets(sha256, filename, size, path, repository, repository_path, commit_hash, verification_source, verified_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(sha256) DO UPDATE SET filename=excluded.filename, size=excluded.size, path=excluded.path, repository=excluded.repository, repository_path=excluded.repository_path, commit_hash=excluded.commit_hash, verification_source=excluded.verification_source, verified_at=excluded.verified_at`, asset.SHA256, asset.Filename, asset.Size, asset.Path, asset.Repository, asset.RepositoryPath, asset.Commit, asset.VerificationSource, asset.VerifiedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	for path, cached := range persisted.Paths {
		if _, err := transaction.Exec(`INSERT INTO path_cache(path, size, mod_time_nano, sha256) VALUES(?, ?, ?, ?) ON CONFLICT(path) DO UPDATE SET size=excluded.size, mod_time_nano=excluded.mod_time_nano, sha256=excluded.sha256`, path, cached.Size, cached.ModTimeNano, cached.SHA256); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func (index *Index) persistAsset(asset Asset) error {
	index.mu.Lock()
	cached := index.paths[asset.Path]
	index.mu.Unlock()
	transaction, err := index.db.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec(`INSERT INTO assets(sha256, filename, size, path, repository, repository_path, commit_hash, verification_source, verified_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(sha256) DO UPDATE SET filename=excluded.filename, size=excluded.size, path=excluded.path, repository=excluded.repository, repository_path=excluded.repository_path, commit_hash=excluded.commit_hash, verification_source=excluded.verification_source, verified_at=excluded.verified_at`, asset.SHA256, asset.Filename, asset.Size, asset.Path, asset.Repository, asset.RepositoryPath, asset.Commit, asset.VerificationSource, asset.VerifiedAt.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err := transaction.Exec(`INSERT INTO path_cache(path, size, mod_time_nano, sha256) VALUES(?, ?, ?, ?) ON CONFLICT(path) DO UPDATE SET size=excluded.size, mod_time_nano=excluded.mod_time_nano, sha256=excluded.sha256`, asset.Path, cached.Size, cached.ModTimeNano, cached.SHA256); err != nil {
		return err
	}
	return transaction.Commit()
}

func (index *Index) Close() error { return index.db.Close() }

func (index *Index) hashFile(path string, info os.FileInfo) (string, error) {
	index.mu.Lock()
	if cached, found := index.paths[path]; found && cached.Size == info.Size() && cached.ModTimeNano == info.ModTime().UnixNano() {
		index.mu.Unlock()
		if err := verifyReadableRegular(path, info); err != nil {
			return "", err
		}
		return cached.SHA256, nil
	}
	if flight, found := index.flights[path]; found {
		index.mu.Unlock()
		<-flight.done
		return flight.hash, flight.err
	}
	flight := &hashFlight{done: make(chan struct{})}
	index.flights[path] = flight
	index.mu.Unlock()
	flight.hash, flight.err = hashPath(path, info)
	index.mu.Lock()
	if flight.err == nil {
		index.paths[path] = cachedPath{Size: info.Size(), ModTimeNano: info.ModTime().UnixNano(), SHA256: flight.hash}
	}
	delete(index.flights, path)
	close(flight.done)
	index.mu.Unlock()
	return flight.hash, flight.err
}

func verifyReadableRegular(path string, expected os.FileInfo) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(expected, opened) || opened.Size() != expected.Size() || !opened.ModTime().Equal(expected.ModTime()) {
		return fmt.Errorf("asset changed while opening")
	}
	return nil
}

func regularFile(path string) (string, os.FileInfo, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("asset must be a regular file")
	}
	return filepath.Clean(absolute), info, nil
}

func hashPath(path string, expected os.FileInfo) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(expected, opened) || opened.Size() != expected.Size() || !opened.ModTime().Equal(expected.ModTime()) {
		return "", fmt.Errorf("asset changed while hashing")
	}
	value, err := hashOpenFile(file)
	if err != nil {
		return "", err
	}
	after, err := os.Lstat(path)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(opened, after) || after.Size() != opened.Size() || !after.ModTime().Equal(opened.ModTime()) {
		return "", fmt.Errorf("asset changed while hashing")
	}
	return value, nil
}

func hashOpenFile(file *os.File) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func cloneAssets(values map[string]Asset) map[string]Asset {
	result := make(map[string]Asset, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
func clonePaths(values map[string]cachedPath) map[string]cachedPath {
	result := make(map[string]cachedPath, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
