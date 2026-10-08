package modelassets

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentIndexingNeverFailsOnALockedDatabase(t *testing.T) {
	index, err := NewIndex(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	directory := t.TempDir()
	var indexers sync.WaitGroup
	errs := make(chan error, 64)
	for worker := range 16 {
		indexers.Go(func() {
			for file := range 4 {
				path := filepath.Join(directory, fmt.Sprintf("model-%d-%d.gguf", worker, file))
				if err := os.WriteFile(path, []byte(path), 0o600); err != nil {
					errs <- err
					return
				}
				if _, err := index.IndexFile(path); err != nil {
					errs <- err
				}
			}
		})
	}
	indexers.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent indexing failed: %v", err)
	}
}
