package routerstore_test

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tensors-router/internal/routerstore"
)

func openWithLegacy(t *testing.T, path string, sources ...routerstore.LegacySource) *routerstore.Handle {
	t.Helper()
	handle, err := routerstore.Open(context.Background(), routerstore.Config{
		Path:          path,
		Modules:       []routerstore.Module{notesModule{}},
		LegacySources: sources,
		Logger:        log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("open router store: %v", err)
	}
	return handle
}

func TestImportCopiesLegacyRowsAndArchivesTheFile(t *testing.T) {
	directory := t.TempDir()
	merged := filepath.Join(directory, "analytics.sqlite")
	legacy := filepath.Join(directory, "notes.sqlite")
	writeLegacyNotesDatabase(t, legacy, map[string]string{"a": "alpha", "b": "beta"}, map[string]int{"hits": 3})

	handle := openWithLegacy(t, merged, routerstore.LegacySource{Module: "notes", Path: legacy})
	defer func() { _ = handle.Close() }()

	if got := scalarInt(t, handle.DB(), `SELECT COUNT(*) FROM notes`); got != 2 {
		t.Fatalf("imported notes = %d, want 2", got)
	}
	if got := scalarString(t, handle.DB(), `SELECT body FROM notes WHERE id = 'a'`); got != "alpha" {
		t.Fatalf("note body = %q, want %q", got, "alpha")
	}
	if got := scalarString(t, handle.DB(), `SELECT tag FROM notes WHERE id = 'a'`); got != "" {
		t.Fatalf("column missing from the legacy file was not defaulted: tag = %q", got)
	}
	if got := scalarInt(t, handle.DB(), `SELECT total FROM note_counters WHERE name = 'hits'`); got != 3 {
		t.Fatalf("counter total = %d, want 3", got)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy file still present after import: %v", err)
	}
	if _, err := os.Stat(legacy + ".migrated"); err != nil {
		t.Fatalf("archived legacy file missing: %v", err)
	}
}

func TestImportAddsCountersToExistingRows(t *testing.T) {
	directory := t.TempDir()
	merged := filepath.Join(directory, "analytics.sqlite")
	legacy := filepath.Join(directory, "notes.sqlite")
	writeLegacyNotesDatabase(t, legacy, nil, map[string]int{"hits": 3})

	seeded := openWithLegacy(t, merged)
	if _, err := seeded.DB().Exec(`INSERT INTO note_counters (name, total) VALUES ('hits', 2)`); err != nil {
		t.Fatalf("seed counter: %v", err)
	}
	if err := seeded.Close(); err != nil {
		t.Fatalf("close seeded store: %v", err)
	}

	handle := openWithLegacy(t, merged, routerstore.LegacySource{Module: "notes", Path: legacy})
	defer func() { _ = handle.Close() }()
	if got := scalarInt(t, handle.DB(), `SELECT total FROM note_counters WHERE name = 'hits'`); got != 5 {
		t.Fatalf("counter total = %d, want 5", got)
	}
}

func TestImportRunsOnceAcrossRestarts(t *testing.T) {
	directory := t.TempDir()
	merged := filepath.Join(directory, "analytics.sqlite")
	legacy := filepath.Join(directory, "notes.sqlite")
	writeLegacyNotesDatabase(t, legacy, map[string]string{"a": "alpha"}, map[string]int{"hits": 3})

	first := openWithLegacy(t, merged, routerstore.LegacySource{Module: "notes", Path: legacy})
	if err := first.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	second := openWithLegacy(t, merged, routerstore.LegacySource{Module: "notes", Path: legacy})
	defer func() { _ = second.Close() }()
	if got := scalarInt(t, second.DB(), `SELECT total FROM note_counters WHERE name = 'hits'`); got != 3 {
		t.Fatalf("counter total = %d after restart, want 3", got)
	}
	if got := scalarInt(t, second.DB(), `SELECT COUNT(*) FROM routerstore_legacy_imports`); got != 1 {
		t.Fatalf("ledger rows = %d, want 1", got)
	}
}

func TestImportSkipsRecordedSourceThatSurvivedArchiving(t *testing.T) {
	directory := t.TempDir()
	merged := filepath.Join(directory, "analytics.sqlite")
	legacy := filepath.Join(directory, "notes.sqlite")
	writeLegacyNotesDatabase(t, legacy, nil, map[string]int{"hits": 3})

	first := openWithLegacy(t, merged, routerstore.LegacySource{Module: "notes", Path: legacy})
	if err := first.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}
	writeLegacyNotesDatabase(t, legacy, nil, map[string]int{"hits": 3})

	second := openWithLegacy(t, merged, routerstore.LegacySource{Module: "notes", Path: legacy})
	defer func() { _ = second.Close() }()
	if got := scalarInt(t, second.DB(), `SELECT total FROM note_counters WHERE name = 'hits'`); got != 3 {
		t.Fatalf("counter total = %d, want 3", got)
	}
	warnings := second.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "already imported") {
		t.Fatalf("warnings = %v, want one already-imported warning", warnings)
	}
}

func TestImportFailureLeavesLegacyFileUntouchedAndContinues(t *testing.T) {
	directory := t.TempDir()
	merged := filepath.Join(directory, "analytics.sqlite")
	broken := filepath.Join(directory, "broken.sqlite")
	healthy := filepath.Join(directory, "notes.sqlite")
	if err := os.WriteFile(broken, []byte("this is not a sqlite database"), 0o600); err != nil {
		t.Fatalf("write broken legacy file: %v", err)
	}
	writeLegacyNotesDatabase(t, healthy, map[string]string{"a": "alpha"}, nil)

	handle := openWithLegacy(t, merged,
		routerstore.LegacySource{Module: "notes", Path: broken},
		routerstore.LegacySource{Module: "notes", Path: healthy},
	)
	defer func() { _ = handle.Close() }()

	if got := scalarInt(t, handle.DB(), `SELECT COUNT(*) FROM notes`); got != 1 {
		t.Fatalf("imported notes = %d, want 1", got)
	}
	if _, err := os.Stat(broken); err != nil {
		t.Fatalf("broken legacy file was not left in place: %v", err)
	}
	if got := scalarInt(t, handle.DB(), `SELECT COUNT(*) FROM routerstore_legacy_imports WHERE source = ?`, broken); got != 0 {
		t.Fatalf("failed import was recorded in the ledger")
	}
	warnings := handle.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "import failed") {
		t.Fatalf("warnings = %v, want one import-failure warning", warnings)
	}
}

func TestImportSkipsTheMergedFileItself(t *testing.T) {
	directory := t.TempDir()
	merged := filepath.Join(directory, "analytics.sqlite")

	seeded := openWithLegacy(t, merged)
	if _, err := seeded.DB().Exec(`INSERT INTO note_counters (name, total) VALUES ('hits', 2)`); err != nil {
		t.Fatalf("seed counter: %v", err)
	}
	if err := seeded.Close(); err != nil {
		t.Fatalf("close seeded store: %v", err)
	}

	handle := openWithLegacy(t, merged, routerstore.LegacySource{Module: "notes", Path: merged})
	defer func() { _ = handle.Close() }()
	if got := scalarInt(t, handle.DB(), `SELECT total FROM note_counters WHERE name = 'hits'`); got != 2 {
		t.Fatalf("counter total = %d, want 2", got)
	}
	if got := scalarInt(t, handle.DB(), `SELECT COUNT(*) FROM routerstore_legacy_imports`); got != 0 {
		t.Fatalf("the merged file was imported into itself")
	}
	if _, err := os.Stat(merged + ".migrated"); !os.IsNotExist(err) {
		t.Fatalf("the merged file was archived: %v", err)
	}
}
