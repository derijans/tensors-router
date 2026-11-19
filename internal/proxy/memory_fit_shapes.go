package proxy

import (
	"os"
	"sync"
	"time"

	"tensors-router/internal/gguf"
	"tensors-router/internal/memoryfit"
)

const modelShapeCacheSize = 64

type modelShape struct {
	shape        memoryfit.ModelShape
	architecture string
}

type modelShapeEntry struct {
	size     int64
	modified time.Time
	value    modelShape
	usedAt   time.Time
}

type modelShapeCache struct {
	mu      sync.Mutex
	entries map[string]modelShapeEntry
	read    func(path string) (gguf.ModelFile, error)
}

func newModelShapeCache() *modelShapeCache {
	return &modelShapeCache{entries: map[string]modelShapeEntry{}, read: gguf.ReadModelFile}
}

func (cache *modelShapeCache) shapeOf(path string) (modelShape, bool) {
	if path == "" {
		return modelShape{}, false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return modelShape{}, false
	}
	cache.mu.Lock()
	entry, cached := cache.entries[path]
	cache.mu.Unlock()
	if cached && entry.size == info.Size() && entry.modified.Equal(info.ModTime()) {
		cache.touch(path)
		return entry.value, true
	}
	model, err := cache.read(path)
	if err != nil {
		return modelShape{}, false
	}
	value := shapeFromModelFile(model)
	cache.store(path, modelShapeEntry{size: info.Size(), modified: info.ModTime(), value: value})
	return value, true
}

func shapeFromModelFile(model gguf.ModelFile) modelShape {
	kv, kvKnown := model.Metadata.KVShape()
	return modelShape{
		architecture: model.Metadata.Architecture,
		shape: memoryfit.ModelShape{
			KV:         kv,
			KVKnown:    kvKnown,
			BlockCount: model.Metadata.BlockCount,
			FileMB:     float64(model.FileBytes) / (1024 * 1024),
		},
	}
}

func (cache *modelShapeCache) touch(path string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if entry, ok := cache.entries[path]; ok {
		entry.usedAt = time.Now()
		cache.entries[path] = entry
	}
}

func (cache *modelShapeCache) store(path string, entry modelShapeEntry) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry.usedAt = time.Now()
	cache.entries[path] = entry
	for len(cache.entries) > modelShapeCacheSize {
		cache.evictOldestLocked()
	}
}

func (cache *modelShapeCache) evictOldestLocked() {
	var oldestPath string
	var oldest time.Time
	for path, entry := range cache.entries {
		if oldestPath == "" || entry.usedAt.Before(oldest) {
			oldestPath, oldest = path, entry.usedAt
		}
	}
	delete(cache.entries, oldestPath)
}
