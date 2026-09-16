package routinggroups

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"

	"tensors-router/internal/routerstore/routerstoretest"
)

func writeDatabase(t *testing.T, path string, statements ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func symmetricGroupSchemaV3() []string {
	return []string{
		`CREATE TABLE routing_groups (id TEXT PRIMARY KEY NOT NULL, created_at INTEGER NOT NULL DEFAULT (unixepoch()))`,
		`CREATE TABLE routing_group_members (node_id TEXT NOT NULL, image_id TEXT NOT NULL, group_id TEXT NOT NULL, restore_after_borrow INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (node_id, image_id))`,
		`CREATE TABLE routing_text_groups (id TEXT PRIMARY KEY NOT NULL, created_at INTEGER NOT NULL DEFAULT (unixepoch()))`,
		`CREATE TABLE routing_text_group_members (node_id TEXT NOT NULL, model_id TEXT NOT NULL, group_id TEXT NOT NULL, restore_after_borrow INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (node_id, model_id))`,
	}
}

func TestMigrateTurnsEachGroupPairIntoTwoLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.sqlite")
	writeDatabase(t, path, append(symmetricGroupSchemaV3(),
		`INSERT INTO routing_groups(id) VALUES ('group')`,
		`INSERT INTO routing_group_members(node_id, image_id, group_id, restore_after_borrow) VALUES ('master', 'cc-ff', 'group', 0)`,
		`INSERT INTO routing_group_members(node_id, image_id, group_id, restore_after_borrow) VALUES ('slave', 'cc11-ff', 'group', 1)`,
		`INSERT INTO routing_text_groups(id) VALUES ('text')`,
		`INSERT INTO routing_text_group_members(node_id, model_id, group_id) VALUES ('master', 'llama', 'text')`,
		`INSERT INTO routing_text_group_members(node_id, model_id, group_id) VALUES ('slave', 'llama-q8', 'text')`,
	)...)

	handle := routerstoretest.OpenAt(t, path, SchemaModule{})
	store := NewStore(handle.DB(), handle.Reader())

	wantImage := []Link{
		{Owner: masterFF(), Helper: slaveFF(), LoadIfUnloaded: true, RestoreAfterBorrow: true},
		{Owner: slaveFF(), Helper: masterFF(), LoadIfUnloaded: true, RestoreAfterBorrow: false},
	}
	if links := mustLinks(t, store, ImageLane); !reflect.DeepEqual(links, wantImage) {
		t.Fatalf("image links = %+v, want %+v", links, wantImage)
	}
	if links := mustLinks(t, store, TextLane); len(links) != 2 {
		t.Fatalf("text links = %+v, want two", links)
	}
	for _, table := range []string{"routing_groups", "routing_group_members", "routing_text_groups", "routing_text_group_members"} {
		var count int
		if err := handle.Reader().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("table %s survived the migration", table)
		}
	}

	if err := (SchemaModule{}).Migrate(context.Background(), handle.DB()); err != nil {
		t.Fatalf("second migrate call must be idempotent: %v", err)
	}
	if links := mustLinks(t, store, ImageLane); !reflect.DeepEqual(links, wantImage) {
		t.Fatalf("image links after a second migrate = %+v, want %+v", links, wantImage)
	}
}

func TestMigrateConvertsGroupsSavedBeforeTheRestoreColumnExisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.sqlite")
	writeDatabase(t, path,
		`CREATE TABLE routing_groups (id TEXT PRIMARY KEY NOT NULL)`,
		`CREATE TABLE routing_group_members (node_id TEXT NOT NULL, image_id TEXT NOT NULL, group_id TEXT NOT NULL, PRIMARY KEY (node_id, image_id))`,
		`INSERT INTO routing_group_members(node_id, image_id, group_id) VALUES ('master', 'cc-ff', 'group')`,
		`INSERT INTO routing_group_members(node_id, image_id, group_id) VALUES ('slave', 'cc11-ff', 'group')`,
	)
	handle := routerstoretest.OpenAt(t, path, SchemaModule{})
	if links := mustLinks(t, NewStore(handle.DB(), handle.Reader()), ImageLane); len(links) != 2 {
		t.Fatalf("links = %+v, want two", links)
	}
}

func TestImportLegacyConvertsGroupPairsAndToleratesMissingTextTables(t *testing.T) {
	legacyPath := filepath.Join(t.TempDir(), "legacy.sqlite")
	writeDatabase(t, legacyPath,
		`CREATE TABLE routing_groups (id TEXT PRIMARY KEY NOT NULL, created_at INTEGER NOT NULL DEFAULT (unixepoch()))`,
		`CREATE TABLE routing_group_members (node_id TEXT NOT NULL, image_id TEXT NOT NULL, group_id TEXT NOT NULL, PRIMARY KEY (node_id, image_id))`,
		`INSERT INTO routing_group_members(node_id, image_id, group_id) VALUES ('master', 'cc-ff', 'group')`,
		`INSERT INTO routing_group_members(node_id, image_id, group_id) VALUES ('slave', 'cc11-ff', 'group')`,
	)

	handle := routerstoretest.OpenAt(t, filepath.Join(t.TempDir(), "current.sqlite"), SchemaModule{})
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
	if copied != 2 {
		t.Fatalf("copied = %d, want 2 links", copied)
	}
	store := NewStore(handle.DB(), handle.Reader())
	if links := mustLinks(t, store, ImageLane); len(links) != 2 {
		t.Fatalf("image links = %+v, want two", links)
	}
	if links := mustLinks(t, store, TextLane); len(links) != 0 {
		t.Fatalf("text links = %+v, want none", links)
	}
}
