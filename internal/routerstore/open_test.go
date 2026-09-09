package routerstore_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"tensors-router/internal/routerstore"
	"tensors-router/internal/routerstore/routerstoretest"
)

func TestOpenRecordsModuleVersionsAndFileFormat(t *testing.T) {
	handle := routerstoretest.Open(t, notesModule{})
	if got := scalarInt(t, handle.DB(), `SELECT version FROM routerstore_schema_versions WHERE module = 'notes'`); got != 3 {
		t.Fatalf("module version = %d, want 3", got)
	}
	if got := scalarInt(t, handle.DB(), `PRAGMA user_version`); got != 6 {
		t.Fatalf("user_version = %d, want 6", got)
	}
}

func TestOpenNeverLowersFileFormatVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "analytics.sqlite")
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open seed database: %v", err)
	}
	if _, err := seed.Exec(`PRAGMA user_version = 9`); err != nil {
		t.Fatalf("seed user_version: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed database: %v", err)
	}
	handle := routerstoretest.OpenAt(t, path, notesModule{})
	if got := scalarInt(t, handle.DB(), `PRAGMA user_version`); got != 9 {
		t.Fatalf("user_version = %d, want 9", got)
	}
}

func TestOpenEnablesForeignKeysOnBothPools(t *testing.T) {
	handle := routerstoretest.Open(t, notesModule{})
	if got := scalarInt(t, handle.DB(), `PRAGMA foreign_keys`); got != 1 {
		t.Fatalf("writer foreign_keys = %d, want 1", got)
	}
	if got := scalarInt(t, handle.Reader(), `PRAGMA foreign_keys`); got != 1 {
		t.Fatalf("reader foreign_keys = %d, want 1", got)
	}
}

func TestOpenRestrictsDatabaseFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not enforced on windows")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "nested", "analytics.sqlite")
	handle := routerstoretest.OpenAt(t, path, notesModule{})
	if _, err := handle.DB().Exec(`INSERT INTO notes (id, body) VALUES ('a', 'b')`); err != nil {
		t.Fatalf("write note: %v", err)
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat directory: %v", err)
	}
	if parent.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %o, want 700", parent.Mode().Perm())
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		info, err := os.Stat(path + suffix)
		if err != nil {
			t.Fatalf("stat %q: %v", path+suffix, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%q mode = %o, want 600", path+suffix, info.Mode().Perm())
		}
	}
}

func TestOpenRejectsPathTheDriverWouldTruncate(t *testing.T) {
	_, err := routerstore.Open(context.Background(), routerstore.Config{
		Path:    filepath.Join(t.TempDir(), "analytics.sqlite?_pragma=foreign_keys(0)"),
		Modules: []routerstore.Module{notesModule{}},
	})
	if err == nil {
		t.Fatal("open accepted a path containing a question mark")
	}
}

func TestOpenRequiresPath(t *testing.T) {
	if _, err := routerstore.Open(context.Background(), routerstore.Config{}); err == nil {
		t.Fatal("open accepted an empty path")
	}
}
