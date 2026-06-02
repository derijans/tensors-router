package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type HashStore struct {
	path  string
	mu    sync.Mutex
	cache hashCache
	seen  map[string]struct{}
}

type hashCache struct {
	Version int                         `json:"version"`
	Files   map[string]cachedFileRecord `json:"files"`
}

type cachedFileRecord struct {
	Size        int64  `json:"size"`
	ModTimeNano int64  `json:"mod_time_nano"`
	Hash        string `json:"hash"`
}

var pathValueKeys = map[string]struct{}{
	"model":           {},
	"model_param":     {},
	"lora":            {},
	"preloadstory":    {},
	"savedatafile":    {},
	"mmproj":          {},
	"draftmodel":      {},
	"sdmodel":         {},
	"sdupscaler":      {},
	"sdvae":           {},
	"sdt5xxl":         {},
	"sdclip1":         {},
	"sdclip2":         {},
	"sdclipl":         {},
	"sdclipg":         {},
	"sdphotomaker":    {},
	"sdlora":          {},
	"whispermodel":    {},
	"ttsmodel":        {},
	"ttswavtokenizer": {},
	"ttsdir":          {},
	"musicllm":        {},
	"musicembeddings": {},
	"musicdiffusion":  {},
	"musicvae":        {},
	"embeddingsmodel": {},
	"admindir":        {},
	"downloaddir":     {},
	"mcpfile":         {},
	"baseconfig":      {},
}

func NewHashStore(storeDir string) (*HashStore, error) {
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return nil, err
	}
	store := &HashStore{
		path: filepath.Join(storeDir, "hash-cache.json"),
		cache: hashCache{
			Version: 1,
			Files:   map[string]cachedFileRecord{},
		},
		seen: map[string]struct{}{},
	}
	content, err := os.ReadFile(store.path)
	if err == nil {
		var loaded hashCache
		if json.Unmarshal(content, &loaded) == nil && loaded.Files != nil {
			store.cache = loaded
		}
	}
	return store, nil
}

func (store *HashStore) StartScan() {
	store.mu.Lock()
	store.seen = map[string]struct{}{}
	store.mu.Unlock()
}

func (store *HashStore) Save() error {
	store.mu.Lock()
	defer store.mu.Unlock()

	for path := range store.cache.Files {
		if _, ok := store.seen[path]; !ok {
			delete(store.cache.Files, path)
		}
	}

	content, err := json.MarshalIndent(store.cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(store.path, content, 0o644)
}

func (store *HashStore) ModelHash(configContent []byte) string {
	return ModelReferenceHash(configContent, store.HashFile)
}

func (store *HashStore) HashFile(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(absolute)
	if err != nil || info.IsDir() {
		return "", false
	}

	store.mu.Lock()
	store.seen[absolute] = struct{}{}
	cached, ok := store.cache.Files[absolute]
	if ok && cached.Size == info.Size() && cached.ModTimeNano == info.ModTime().UnixNano() {
		hash := cached.Hash
		store.mu.Unlock()
		return hash, true
	}
	store.mu.Unlock()

	hash, err := hashFileContent(absolute)
	if err != nil {
		return "", false
	}

	store.mu.Lock()
	store.cache.Files[absolute] = cachedFileRecord{
		Size:        info.Size(),
		ModTimeNano: info.ModTime().UnixNano(),
		Hash:        hash,
	}
	store.mu.Unlock()
	return hash, true
}

func ConfigHash(content []byte) string {
	var value any
	if err := json.Unmarshal(content, &value); err != nil {
		return hashBytes(content)
	}
	normalized := normalizeConfigValue("", value)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return hashBytes(content)
	}
	return hashBytes(encoded)
}

func ModelReferenceHash(content []byte, hashFile func(string) (string, bool)) string {
	var raw map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		return ""
	}
	entries := make([]string, 0)
	for key, value := range raw {
		if _, ok := pathValueKeys[strings.ToLower(key)]; !ok {
			continue
		}
		for _, path := range stringValues(value) {
			if strings.TrimSpace(path) == "" {
				continue
			}
			hash := ""
			if hashFile != nil {
				if fileHash, ok := hashFile(path); ok {
					hash = fileHash
				}
			}
			if hash == "" {
				hash = "missing:" + filenameStem(path)
			}
			entries = append(entries, strings.ToLower(key)+"="+hash)
		}
	}
	if len(entries) == 0 {
		return ""
	}
	sort.Strings(entries)
	return hashBytes([]byte(strings.Join(entries, "\n")))
}

func normalizeConfigValue(key string, value any) any {
	lowerKey := strings.ToLower(key)
	if _, ok := pathValueKeys[lowerKey]; ok {
		return normalizePathValue(value)
	}

	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for childKey := range typed {
			keys = append(keys, childKey)
		}
		sort.Strings(keys)
		normalized := map[string]any{}
		for _, childKey := range keys {
			normalized[childKey] = normalizeConfigValue(childKey, typed[childKey])
		}
		return normalized
	case []any:
		normalized := make([]any, 0, len(typed))
		for _, item := range typed {
			normalized = append(normalized, normalizeConfigValue("", item))
		}
		return normalized
	case string:
		if looksLikePath(typed) {
			return "<path>"
		}
		return typed
	default:
		return value
	}
}

func normalizePathValue(value any) any {
	switch typed := value.(type) {
	case []any:
		normalized := make([]any, 0, len(typed))
		for _, item := range typed {
			normalized = append(normalized, normalizePathValue(item))
		}
		return normalized
	case string:
		if strings.TrimSpace(typed) == "" {
			return ""
		}
		return "<path>"
	default:
		return value
	}
}

func looksLikePath(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, `:\`) || strings.Contains(trimmed, ":/") {
		return true
	}
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, `\`) {
		return true
	}
	return strings.Contains(trimmed, "/") || strings.Contains(trimmed, `\`)
}

func hashFileContent(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashBytes(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}
