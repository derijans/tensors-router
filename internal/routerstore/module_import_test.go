package routerstore_test

import (
	"context"
	"database/sql"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/loadcapture"
	"tensors-router/internal/loaderrors"
	"tensors-router/internal/routerstore"
	"tensors-router/internal/routinggroups"
)

func routerModules() []routerstore.Module {
	return []routerstore.Module{
		routeranalytics.SchemaModule{},
		loadcapture.SchemaModule{},
		loaderrors.SchemaModule{},
		routinggroups.SchemaModule{},
	}
}

func writeLegacyDatabase(t *testing.T, path string, statements ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer func() { _ = db.Close() }()
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed legacy database %s: %v", filepath.Base(path), err)
		}
	}
}

func openRouterStore(t *testing.T, path string, sources ...routerstore.LegacySource) *routerstore.Handle {
	t.Helper()
	handle, err := routerstore.Open(context.Background(), routerstore.Config{
		Path:          path,
		Modules:       routerModules(),
		LegacySources: sources,
		Logger:        log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("open router store: %v", err)
	}
	return handle
}

// The legacy analytics table predates the image_steps and vram columns, so the
// import copies what the two schemas share and leaves the rest at its default.
func seedLegacyAnalytics(t *testing.T, path string) {
	t.Helper()
	writeLegacyDatabase(t, path,
		`CREATE TABLE analytics_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id TEXT NOT NULL, model_id TEXT NOT NULL, section TEXT NOT NULL,
			backend_mode TEXT NOT NULL, route TEXT NOT NULL, status_code INTEGER NOT NULL,
			success INTEGER NOT NULL, started_at INTEGER NOT NULL, finished_at INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL, total_tokens INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO analytics_events (node_id, model_id, section, backend_mode, route, status_code, success, started_at, finished_at, duration_ms, total_tokens)
			VALUES ('node-a','llm-a','llm','kobold','/v1/chat/*',200,1,10,20,10,7)`,
		`CREATE TABLE analytics_rollups (
			period_kind TEXT NOT NULL, bucket_start INTEGER NOT NULL, node_id TEXT NOT NULL,
			model_id TEXT NOT NULL, section TEXT NOT NULL, backend_mode TEXT NOT NULL, route TEXT NOT NULL,
			request_count INTEGER NOT NULL DEFAULT 0, total_tokens INTEGER NOT NULL DEFAULT 0,
			vram_peak_mb INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (period_kind, bucket_start, node_id, model_id, section, backend_mode, route))`,
		`INSERT INTO analytics_rollups (period_kind, bucket_start, node_id, model_id, section, backend_mode, route, request_count, total_tokens, vram_peak_mb)
			VALUES ('hour', 3600000, 'node-a', 'llm-a', 'llm', 'kobold', '/v1/chat/*', 3, 7, 50)`,
	)
}

func seedLegacyLoadCapture(t *testing.T, path string) {
	t.Helper()
	snapshot := strings.Repeat("a", 64)
	asset := strings.Repeat("b", 64)
	writeLegacyDatabase(t, path,
		`CREATE TABLE snapshots (sha256 TEXT PRIMARY KEY, payload BLOB NOT NULL, created_at INTEGER NOT NULL)`,
		`CREATE TABLE snapshot_assets (snapshot_sha256 TEXT NOT NULL, role TEXT NOT NULL, position INTEGER NOT NULL, sha256 TEXT NOT NULL, PRIMARY KEY (snapshot_sha256, role, position))`,
		`CREATE TABLE attempts (id TEXT PRIMARY KEY, node_id TEXT NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, backend_mode TEXT NOT NULL, runtime TEXT NOT NULL, lane TEXT NOT NULL, snapshot_sha256 TEXT NOT NULL, physical_attempt_id TEXT, started_at INTEGER NOT NULL, finished_at INTEGER NOT NULL DEFAULT 0, duration_ms INTEGER NOT NULL DEFAULT 0, failure_class TEXT NOT NULL DEFAULT '', failure_message TEXT NOT NULL DEFAULT '', captured_bytes INTEGER NOT NULL DEFAULT 0, truncated INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE output_chunks (physical_attempt_id TEXT NOT NULL, sequence INTEGER NOT NULL, stream TEXT NOT NULL, offset_ns INTEGER NOT NULL, payload BLOB NOT NULL, PRIMARY KEY (physical_attempt_id, sequence))`,
		`INSERT INTO snapshots (sha256, payload, created_at) VALUES ('`+snapshot+`', X'7B7D', 5)`,
		`INSERT INTO snapshot_assets (snapshot_sha256, role, position, sha256) VALUES ('`+snapshot+`', 'model_param', 0, '`+asset+`')`,
		`INSERT INTO attempts (id, node_id, kind, status, backend_mode, runtime, lane, snapshot_sha256, physical_attempt_id, started_at)
			VALUES ('attempt-1', 'node-a', 'physical', 'loading', 'kobold', 'koboldcpp', 'llm', '`+snapshot+`', NULL, 11)`,
		`INSERT INTO output_chunks (physical_attempt_id, sequence, stream, offset_ns, payload)
			VALUES ('attempt-1', 1, 'stdout', 1000, X'6C6F6164')`,
	)
}

func seedLegacyLoadErrors(t *testing.T, path string) {
	t.Helper()
	writeLegacyDatabase(t, path,
		`CREATE TABLE load_errors (id TEXT PRIMARY KEY, fingerprint TEXT NOT NULL UNIQUE, first_seen_at INTEGER NOT NULL, last_seen_at INTEGER NOT NULL, occurrences INTEGER NOT NULL DEFAULT 1, node_id TEXT NOT NULL DEFAULT '', model_id TEXT NOT NULL DEFAULT '', config_name TEXT NOT NULL DEFAULT '', backend TEXT NOT NULL DEFAULT '', backend_mode TEXT NOT NULL DEFAULT '', phase TEXT NOT NULL, severity TEXT NOT NULL DEFAULT 'error', source TEXT NOT NULL DEFAULT '', message TEXT NOT NULL, exit_error TEXT NOT NULL DEFAULT '', output TEXT NOT NULL DEFAULT '', truncated INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO load_errors (id, fingerprint, first_seen_at, last_seen_at, occurrences, phase, message)
			VALUES ('error-1', 'print-1', 100, 200, 4, 'preload', 'model failed to load')`,
	)
}

func seedLegacyRoutingGroups(t *testing.T, path string) {
	t.Helper()
	writeLegacyDatabase(t, path,
		`CREATE TABLE routing_groups (id TEXT PRIMARY KEY NOT NULL, created_at INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE routing_group_members (node_id TEXT NOT NULL, image_id TEXT NOT NULL, group_id TEXT NOT NULL, PRIMARY KEY (node_id, image_id))`,
		`INSERT INTO routing_groups (id, created_at) VALUES ('group-1', 42)`,
		`INSERT INTO routing_group_members (node_id, image_id, group_id) VALUES ('node-a', 'sdxl', 'group-1')`,
		`INSERT INTO routing_group_members (node_id, image_id, group_id) VALUES ('node-b', 'sdxl-q8', 'group-1')`,
	)
}

func TestRouterModulesImportEveryLegacyDatabase(t *testing.T) {
	directory := t.TempDir()
	merged := filepath.Join(directory, "analytics.sqlite")
	legacy := map[string]string{
		"analytics":     filepath.Join(directory, "old-analytics.sqlite"),
		"loadcapture":   filepath.Join(directory, "load-captures.sqlite"),
		"loaderrors":    filepath.Join(directory, "load-errors.sqlite"),
		"routinggroups": filepath.Join(directory, "routing-groups.sqlite"),
	}
	seedLegacyAnalytics(t, legacy["analytics"])
	seedLegacyLoadCapture(t, legacy["loadcapture"])
	seedLegacyLoadErrors(t, legacy["loaderrors"])
	seedLegacyRoutingGroups(t, legacy["routinggroups"])

	sources := make([]routerstore.LegacySource, 0, len(legacy))
	for module, path := range legacy {
		sources = append(sources, routerstore.LegacySource{Module: module, Path: path})
	}
	handle := openRouterStore(t, merged, sources...)
	defer func() { _ = handle.Close() }()
	if warnings := handle.Warnings(); len(warnings) != 0 {
		t.Fatalf("import warned: %v", warnings)
	}

	reader := handle.Reader()
	if got := scalarString(t, reader, `SELECT model_id FROM analytics_events`); got != "llm-a" {
		t.Fatalf("analytics event model = %q, want %q", got, "llm-a")
	}
	if got := scalarInt(t, reader, `SELECT image_steps FROM analytics_events`); got != 0 {
		t.Fatalf("column missing from the legacy analytics table was not defaulted: %d", got)
	}
	if got := scalarInt(t, reader, `SELECT request_count FROM analytics_rollups`); got != 3 {
		t.Fatalf("rollup request count = %d, want 3", got)
	}
	if got := scalarString(t, reader, `SELECT id FROM load_capture_attempts`); got != "attempt-1" {
		t.Fatalf("capture attempt = %q, want %q", got, "attempt-1")
	}
	if got := scalarInt(t, reader, `SELECT COUNT(*) FROM load_capture_snapshot_assets`); got != 1 {
		t.Fatalf("capture snapshot assets = %d, want 1", got)
	}
	var payload []byte
	if err := reader.QueryRow(`SELECT payload FROM load_capture_output_chunks WHERE physical_attempt_id = 'attempt-1'`).Scan(&payload); err != nil {
		t.Fatalf("read imported output chunk: %v", err)
	}
	if string(payload) != "load" {
		t.Fatalf("output chunk payload = %q, want %q", payload, "load")
	}
	if got := scalarInt(t, reader, `SELECT occurrences FROM load_errors WHERE fingerprint = 'print-1'`); got != 4 {
		t.Fatalf("load error occurrences = %d, want 4", got)
	}
	if got := scalarInt(t, reader, `SELECT COUNT(*) FROM routing_group_members WHERE group_id = 'group-1'`); got != 2 {
		t.Fatalf("routing group members = %d, want 2", got)
	}
	for module, path := range legacy {
		if _, err := os.Stat(path + ".migrated"); err != nil {
			t.Fatalf("%s legacy file was not archived: %v", module, err)
		}
	}
}

func TestAnalyticsRollupImportAddsOverlappingBuckets(t *testing.T) {
	directory := t.TempDir()
	merged := filepath.Join(directory, "analytics.sqlite")
	legacyPath := filepath.Join(directory, "old-analytics.sqlite")
	seedLegacyAnalytics(t, legacyPath)

	seeded := openRouterStore(t, merged)
	if _, err := seeded.DB().Exec(`INSERT INTO analytics_rollups (period_kind, bucket_start, node_id, model_id, section, backend_mode, route, request_count, total_tokens, vram_peak_mb)
		VALUES ('hour', 3600000, 'node-a', 'llm-a', 'llm', 'kobold', '/v1/chat/*', 2, 5, 100)`); err != nil {
		t.Fatalf("seed merged rollup: %v", err)
	}
	if err := seeded.Close(); err != nil {
		t.Fatalf("close seeded store: %v", err)
	}

	handle := openRouterStore(t, merged, routerstore.LegacySource{Module: "analytics", Path: legacyPath})
	defer func() { _ = handle.Close() }()
	if got := scalarInt(t, handle.Reader(), `SELECT request_count FROM analytics_rollups`); got != 5 {
		t.Fatalf("request count = %d, want 5", got)
	}
	if got := scalarInt(t, handle.Reader(), `SELECT total_tokens FROM analytics_rollups`); got != 12 {
		t.Fatalf("total tokens = %d, want 12", got)
	}
	if got := scalarInt(t, handle.Reader(), `SELECT vram_peak_mb FROM analytics_rollups`); got != 100 {
		t.Fatalf("vram peak = %d, want the higher reading 100", got)
	}
}

func TestImportedLoadingAttemptIsReconciledWhenCaptureStarts(t *testing.T) {
	directory := t.TempDir()
	merged := filepath.Join(directory, "analytics.sqlite")
	legacyPath := filepath.Join(directory, "load-captures.sqlite")
	seedLegacyLoadCapture(t, legacyPath)

	handle := openRouterStore(t, merged, routerstore.LegacySource{Module: "loadcapture", Path: legacyPath})
	defer func() { _ = handle.Close() }()
	if _, err := loadcapture.NewStore(loadcapture.StoreConfig{NodeID: "node-a", DB: handle.DB(), ReadDB: handle.Reader(), Logger: log.New(io.Discard, "", 0)}); err != nil {
		t.Fatalf("start load capture store: %v", err)
	}
	if got := scalarString(t, handle.Reader(), `SELECT status FROM load_capture_attempts WHERE id = 'attempt-1'`); got != string(loadcapture.StatusInterrupted) {
		t.Fatalf("imported attempt status = %q, want %q", got, loadcapture.StatusInterrupted)
	}
}

func TestSubsystemsShareOneHandleWithoutRacing(t *testing.T) {
	handle := openRouterStore(t, filepath.Join(t.TempDir(), "analytics.sqlite"))
	defer func() { _ = handle.Close() }()
	discard := log.New(io.Discard, "", 0)

	analyticsStore, err := routeranalytics.NewStore(routeranalytics.StoreConfig{NodeID: "node-a", DB: handle.DB(), ReadDB: handle.Reader(), FlushInterval: time.Hour, Logger: discard})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = analyticsStore.Close(context.Background()) }()
	captureStore, err := loadcapture.NewStore(loadcapture.StoreConfig{NodeID: "node-a", DB: handle.DB(), ReadDB: handle.Reader(), Logger: discard})
	if err != nil {
		t.Fatal(err)
	}
	errorStore, err := loaderrors.NewStore(loaderrors.StoreConfig{NodeID: "node-a", DB: handle.DB(), ReadDB: handle.Reader()})
	if err != nil {
		t.Fatal(err)
	}
	groupStore := routinggroups.NewStore(handle.DB(), handle.Reader())

	const rounds = 8
	failures := make(chan error, 4*rounds)
	var waiting sync.WaitGroup
	waiting.Add(4)
	go func() {
		defer waiting.Done()
		now := time.Now().UTC()
		for round := 0; round < rounds; round++ {
			analyticsStore.Record(routeranalytics.Event{ModelID: "llm-a", Section: routeranalytics.SectionLLM, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now})
			if err := analyticsStore.Flush(context.Background()); err != nil {
				failures <- err
				return
			}
		}
	}()
	go func() {
		defer waiting.Done()
		for round := 0; round < rounds; round++ {
			snapshot := loadcapture.Snapshot{SHA256: strings.Repeat(strconv.Itoa(round), 64), JSON: []byte(`{}`)}
			attempt, err := captureStore.BeginPhysical(context.Background(), snapshot, "kobold", "koboldcpp", "llm")
			if err != nil {
				failures <- err
				return
			}
			if err := captureStore.CompletePhysical(context.Background(), attempt, nil, loadcapture.Capture{}, nil); err != nil {
				failures <- err
				return
			}
		}
	}()
	go func() {
		defer waiting.Done()
		for round := 0; round < rounds; round++ {
			if err := errorStore.Record(context.Background(), loaderrors.RecordInput{Phase: loaderrors.PhasePreload, Source: "test", Message: "failure " + strconv.Itoa(round)}); err != nil {
				failures <- err
				return
			}
		}
	}()
	go func() {
		defer waiting.Done()
		for round := 0; round < rounds; round++ {
			if _, err := groupStore.SetGroup(context.Background(), routinggroups.Member{NodeID: "node-a", ImageID: "sdxl"}, []routinggroups.Member{{NodeID: "node-b", ImageID: "sdxl-q8"}}); err != nil {
				failures <- err
				return
			}
			if _, err := groupStore.Groups(context.Background()); err != nil {
				failures <- err
				return
			}
		}
	}()
	waiting.Wait()
	close(failures)
	for err := range failures {
		t.Fatalf("shared handle failed under concurrent use: %v", err)
	}

	if got := scalarInt(t, handle.Reader(), `SELECT COUNT(*) FROM analytics_events`); got != rounds {
		t.Fatalf("analytics events = %d, want %d", got, rounds)
	}
	if got := scalarInt(t, handle.Reader(), `SELECT COUNT(*) FROM load_capture_attempts`); got != rounds {
		t.Fatalf("capture attempts = %d, want %d", got, rounds)
	}
	if got := scalarInt(t, handle.Reader(), `SELECT occurrences FROM load_errors`); got != rounds {
		t.Fatalf("load error occurrences = %d, want %d", got, rounds)
	}
	if got := scalarInt(t, handle.Reader(), `SELECT COUNT(*) FROM routing_group_members`); got != 2 {
		t.Fatalf("routing group members = %d, want 2", got)
	}
}
