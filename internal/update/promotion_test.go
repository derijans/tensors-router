//go:build !windows

package update

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestArchivePromotionNeverExposesMissingOrPartialExecutable(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "installed")
	stagingDir := filepath.Join(root, "staging")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldContent := bytes.Repeat([]byte("o"), 256*1024)
	newContent := bytes.Repeat([]byte("n"), 256*1024)
	binaryName := "server"
	targetBinary := filepath.Join(targetDir, binaryName)
	if err := os.WriteFile(targetBinary, oldContent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, binaryName), newContent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "runtime.bin"), []byte("runtime"), 0o644); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	readerResult := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				readerResult <- nil
				return
			default:
			}
			content, err := os.ReadFile(targetBinary)
			if err != nil {
				readerResult <- err
				return
			}
			if !bytes.Equal(content, oldContent) && !bytes.Equal(content, newContent) {
				readerResult <- &unexpectedExecutableContentError{length: len(content)}
				return
			}
		}
	}()
	promotion, err := promoteArchiveTree(stagingDir, targetDir, binaryName)
	if err == nil {
		err = promotion.Commit()
	}
	close(stop)
	if readerErr := <-readerResult; readerErr != nil {
		t.Fatal(readerErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(targetBinary)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, newContent) {
		t.Fatal("archive promotion did not install the complete replacement executable")
	}
}

type unexpectedExecutableContentError struct {
	length int
}

func (err *unexpectedExecutableContentError) Error() string {
	return "archive promotion exposed partial executable with length " + fmt.Sprint(err.length)
}
