package routerstore_test

import (
	"context"
	"database/sql"
	"testing"

	"tensors-router/internal/routerstore"

	_ "modernc.org/sqlite"
)

type notesModule struct{}

func (notesModule) Name() string { return "notes" }

func (notesModule) Version() int { return 3 }

func (notesModule) Migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS notes (id TEXT PRIMARY KEY, body TEXT NOT NULL DEFAULT '', tag TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS note_counters (name TEXT PRIMARY KEY, total INTEGER NOT NULL DEFAULT 0)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (notesModule) ImportLegacy(ctx context.Context, tx *sql.Tx, legacySchema string) (int64, error) {
	notes, err := routerstore.CopyRows(ctx, tx, routerstore.CopySpec{
		LegacySchema: legacySchema,
		LegacyTable:  "notes",
		Table:        "notes",
		Columns:      []string{"id", "body", "tag"},
		Conflict:     func([]string) string { return `ON CONFLICT(id) DO NOTHING` },
	})
	if err != nil {
		return 0, err
	}
	counters, err := routerstore.CopyRows(ctx, tx, routerstore.CopySpec{
		LegacySchema: legacySchema,
		LegacyTable:  "note_counters",
		Table:        "note_counters",
		Columns:      []string{"name", "total"},
		Conflict: func([]string) string {
			return `ON CONFLICT(name) DO UPDATE SET total = note_counters.total + excluded.total`
		},
	})
	if err != nil {
		return 0, err
	}
	return notes + counters, nil
}

func writeLegacyNotesDatabase(t *testing.T, path string, notes map[string]string, counters map[string]int) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer func() { _ = db.Close() }()
	statements := []string{
		`CREATE TABLE notes (id TEXT PRIMARY KEY, body TEXT NOT NULL)`,
		`CREATE TABLE note_counters (name TEXT PRIMARY KEY, total INTEGER NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create legacy schema: %v", err)
		}
	}
	for id, body := range notes {
		if _, err := db.Exec(`INSERT INTO notes (id, body) VALUES (?, ?)`, id, body); err != nil {
			t.Fatalf("seed legacy note: %v", err)
		}
	}
	for name, total := range counters {
		if _, err := db.Exec(`INSERT INTO note_counters (name, total) VALUES (?, ?)`, name, total); err != nil {
			t.Fatalf("seed legacy counter: %v", err)
		}
	}
}

func scalarInt(t *testing.T, db *sql.DB, query string, arguments ...any) int {
	t.Helper()
	var value int
	if err := db.QueryRow(query, arguments...).Scan(&value); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return value
}

func scalarString(t *testing.T, db *sql.DB, query string, arguments ...any) string {
	t.Helper()
	var value string
	if err := db.QueryRow(query, arguments...).Scan(&value); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return value
}
