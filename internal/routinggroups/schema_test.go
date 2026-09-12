package routinggroups

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"tensors-router/internal/routerstore/routerstoretest"
)

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

func TestMigrateAddsRestoreColumnToAnExistingV2Database(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE routing_groups (id TEXT PRIMARY KEY NOT NULL, created_at INTEGER NOT NULL DEFAULT (unixepoch()))`,
		`CREATE TABLE routing_group_members (node_id TEXT NOT NULL, image_id TEXT NOT NULL, group_id TEXT NOT NULL, PRIMARY KEY (node_id, image_id))`,
		`CREATE TABLE routing_text_groups (id TEXT PRIMARY KEY NOT NULL, created_at INTEGER NOT NULL DEFAULT (unixepoch()))`,
		`CREATE TABLE routing_text_group_members (node_id TEXT NOT NULL, model_id TEXT NOT NULL, group_id TEXT NOT NULL, PRIMARY KEY (node_id, model_id))`,
		`INSERT INTO routing_groups(id) VALUES ('node-a\x00sdxl')`,
		`INSERT INTO routing_group_members(node_id, image_id, group_id) VALUES ('node-a', 'sdxl', 'node-a\x00sdxl')`,
		`INSERT INTO routing_group_members(node_id, image_id, group_id) VALUES ('node-b', 'sdxl-alt', 'node-a\x00sdxl')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	if err := (SchemaModule{}).Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := (SchemaModule{}).Migrate(ctx, db); err != nil {
		t.Fatalf("second migrate call must be idempotent: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	handle := routerstoretest.OpenAt(t, path, SchemaModule{})
	store := NewStore(handle.DB(), handle.Reader())
	group, found, err := store.Group(ctx, Member{NodeID: "node-a", ImageID: "sdxl"})
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("pre-migration group must survive the column add")
	}
	for _, member := range group.Members {
		if member.RestoreAfterBorrow {
			t.Fatalf("member %+v: pre-existing row must default restore_after_borrow to false", member)
		}
	}
}
