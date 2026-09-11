package routinggroups

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"tensors-router/internal/routerstore/routerstoretest"
)

// TestImportLegacyWithoutTextTablesCopiesNothing pins that a v1 legacy database
// — one predating the text tables entirely — imports cleanly: the image rows
// copy over and the text row count is exactly zero, never an error.
func TestImportLegacyWithoutTextTablesCopiesNothing(t *testing.T) {
	legacyPath := filepath.Join(t.TempDir(), "legacy.sqlite")
	legacyDB, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE routing_groups (id TEXT PRIMARY KEY NOT NULL, created_at INTEGER NOT NULL DEFAULT (unixepoch()))`,
		`CREATE TABLE routing_group_members (node_id TEXT NOT NULL, image_id TEXT NOT NULL, group_id TEXT NOT NULL, PRIMARY KEY (node_id, image_id))`,
		`INSERT INTO routing_groups(id) VALUES ('node-a\x00sdxl')`,
		`INSERT INTO routing_group_members(node_id, image_id, group_id) VALUES ('node-a', 'sdxl', 'node-a\x00sdxl')`,
		`INSERT INTO routing_group_members(node_id, image_id, group_id) VALUES ('node-b', 'sdxl-alt', 'node-a\x00sdxl')`,
	} {
		if _, err := legacyDB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatal(err)
	}

	handle := routerstoretest.OpenAt(t, filepath.Join(t.TempDir(), "current.sqlite"), SchemaModule{})
	store := NewStore(handle.DB(), handle.Reader())
	ctx := context.Background()

	if _, err := handle.DB().ExecContext(ctx, `ATTACH DATABASE ? AS legacy_import`, legacyPath); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = handle.DB().ExecContext(context.Background(), `DETACH DATABASE legacy_import`) }()

	tx, err := handle.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := SchemaModule{}.ImportLegacy(ctx, tx, "legacy_import")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if copied != 3 {
		t.Fatalf("copied = %d, want 3 (one group row, two member rows)", copied)
	}

	imageGroups, err := store.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(imageGroups) != 1 || len(imageGroups[0].Members) != 2 {
		t.Fatalf("unexpected imported image groups %+v", imageGroups)
	}
	textGroups, err := store.TextGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(textGroups) != 0 {
		t.Fatalf("text groups = %d, want 0 for a legacy database with no text tables", len(textGroups))
	}
}
