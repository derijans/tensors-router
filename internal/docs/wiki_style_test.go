package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const emDash = "—"

func documentationTargets() []string {
	repoRoot := filepath.Join("..", "..")
	return []string{
		filepath.Join(repoRoot, "README.md"),
		filepath.Join(repoRoot, "docs", "wiki"),
	}
}

func TestDocumentationHasNoEmDash(t *testing.T) {
	for _, target := range documentationTargets() {
		info, err := os.Stat(target)
		if err != nil {
			t.Fatalf("stat %s: %v", target, err)
		}
		if info.IsDir() {
			for _, markdown := range markdownFilesUnder(t, target) {
				assertNoEmDash(t, markdown)
			}
			continue
		}
		assertNoEmDash(t, target)
	}
}

func markdownFilesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	walk := func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			found = append(found, path)
		}
		return nil
	}
	if err := filepath.WalkDir(dir, walk); err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return found
}

func assertNoEmDash(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for offset, line := range strings.Split(string(content), "\n") {
		if strings.Contains(line, emDash) {
			t.Errorf("%s:%d uses an em dash; rewrite with a comma, parentheses, or a colon", path, offset+1)
		}
	}
}
