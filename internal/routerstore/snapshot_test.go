package routerstore_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"tensors-router/internal/routerstore/routerstoretest"
)

func TestSnapshotHoldsEveryTableAndRowOfTheLiveStore(t *testing.T) {
	handle := routerstoretest.Open(t, notesModule{})
	for _, statement := range []string{
		`INSERT INTO notes (id, body) VALUES ('a', 'first'), ('b', 'second')`,
		`INSERT INTO note_counters (name, total) VALUES ('hits', 7)`,
	} {
		if _, err := handle.DB().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	path, cleanup, err := handle.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	snapshot, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	if got := scalarInt(t, snapshot, `SELECT COUNT(*) FROM notes`); got != 2 {
		t.Fatalf("snapshot notes = %d, want 2", got)
	}
	if got := scalarInt(t, snapshot, `SELECT total FROM note_counters WHERE name = 'hits'`); got != 7 {
		t.Fatalf("snapshot counter = %d, want 7", got)
	}
	if got := scalarInt(t, snapshot, `SELECT version FROM routerstore_schema_versions WHERE module = 'notes'`); got != 3 {
		t.Fatalf("snapshot lost the module version registry: %d", got)
	}
}

func TestSnapshotDoesNotFollowLaterWrites(t *testing.T) {
	handle := routerstoretest.Open(t, notesModule{})
	if _, err := handle.DB().Exec(`INSERT INTO notes (id, body) VALUES ('a', 'first')`); err != nil {
		t.Fatal(err)
	}
	path, cleanup, err := handle.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := handle.DB().Exec(`INSERT INTO notes (id, body) VALUES ('b', 'second')`); err != nil {
		t.Fatal(err)
	}

	snapshot, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	if got := scalarInt(t, snapshot, `SELECT COUNT(*) FROM notes`); got != 1 {
		t.Fatalf("snapshot notes = %d, want the 1 row present when it was taken", got)
	}
}

func TestSnapshotCleanupRemovesTheFileAndLeavesTheStoreIntact(t *testing.T) {
	handle := routerstoretest.Open(t, notesModule{})
	path, cleanup, err := handle.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Dir(handle.Path()) {
		t.Fatalf("snapshot %q is not beside the store %q", path, handle.Path())
	}

	cleanup()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("snapshot file survived cleanup: %v", err)
	}
	if got := scalarInt(t, handle.DB(), `SELECT COUNT(*) FROM notes`); got != 0 {
		t.Fatalf("live store changed: %d notes", got)
	}
}
