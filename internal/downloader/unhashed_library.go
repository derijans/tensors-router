package downloader

import (
	"sort"
	"sync"
)

type UnhashedFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type unhashedLibrary struct {
	mu    sync.Mutex
	files map[string]int64
}

func (library *unhashedLibrary) replace(files []UnhashedFile) {
	sizes := make(map[string]int64, len(files))
	for _, file := range files {
		sizes[file.Path] = file.Size
	}
	library.mu.Lock()
	library.files = sizes
	library.mu.Unlock()
}

func (library *unhashedLibrary) forget(path string) {
	library.mu.Lock()
	delete(library.files, path)
	library.mu.Unlock()
}

func (library *unhashedLibrary) list() []UnhashedFile {
	library.mu.Lock()
	files := make([]UnhashedFile, 0, len(library.files))
	for path, size := range library.files {
		files = append(files, UnhashedFile{Path: path, Size: size})
	}
	library.mu.Unlock()
	sort.Slice(files, func(left int, right int) bool { return files[left].Path < files[right].Path })
	return files
}
