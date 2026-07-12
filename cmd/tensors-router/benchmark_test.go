package main

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/config"
)

func TestParseBenchmarkCommand(t *testing.T) {
	command, err := parseBenchmarkCommand([]string{
		"--model", "alpha",
		"--type", "section",
		"--sections", "runtime,llm",
		"--node", "node-a",
		"--url", "http://router",
		"--token", "secret",
		"--json",
		"--timeout", "60",
		"--iterations", "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if command.URL != "http://router" || command.Token != "secret" || !command.JSON {
		t.Fatalf("unexpected command %#v", command)
	}
	if command.Request.ModelID != "alpha" || command.Request.NodeID != "node-a" || command.Request.Type != routerbenchmark.TypeSection {
		t.Fatalf("unexpected request %#v", command.Request)
	}
	if len(command.Request.Sections) != 2 || command.Request.Sections[0] != "runtime" || command.Request.Sections[1] != "llm" {
		t.Fatalf("unexpected sections %#v", command.Request.Sections)
	}
	if command.Request.TimeoutSeconds != 60 || command.Request.Iterations != 2 {
		t.Fatalf("unexpected limits %#v", command.Request)
	}
}

func TestParseBenchmarkCommandRequiresModel(t *testing.T) {
	if _, err := parseBenchmarkCommand([]string{}); err == nil {
		t.Fatal("expected model requirement")
	}
}

func TestBackendProcessConfigsUseBackendDiskLogToggle(t *testing.T) {
	cfg := config.Defaults()
	cfg.Logging.Mode = config.LoggingModeQuiet

	if koboldProcessConfig(cfg).Logging || llamaProcessConfig(cfg).Logging || sdcppProcessConfig(cfg).Logging {
		t.Fatalf("backend process logging should ignore router logging toggle")
	}

	cfg.Logging.BackendLogsToDisk = true

	if !koboldProcessConfig(cfg).Logging || !llamaProcessConfig(cfg).Logging || !sdcppProcessConfig(cfg).Logging {
		t.Fatalf("backend process logging should follow backend disk log toggle")
	}
}

func TestQuietLoggingKeepsAnalyticsUsageCollectionEnabled(t *testing.T) {
	cfg := config.Defaults()
	cfg.Logging.Mode = config.LoggingModeQuiet
	cfg.Logging.BackendLogsToDisk = false
	cfg.Analytics.Enabled = true
	cfg.Analytics.DatabasePath = filepath.Join(t.TempDir(), "analytics.sqlite")
	_, serveLogger := configuredLoggers(cfg.Logging.Mode)
	store, err := newAnalyticsStore(cfg, serveLogger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	now := time.Now()
	event := routeranalytics.Event{
		ModelID:    "llm-a",
		Section:    routeranalytics.SectionLLM,
		StartedAt:  now,
		StatusCode: 200,
		Success:    true,
	}
	observer := routeranalytics.NewResponseObserver(store, event, "application/json", io.NopCloser(strings.NewReader(`{"usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}`)))
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}

	response, err := store.Query(context.Background(), routeranalytics.Query{Period: routeranalytics.PeriodAll})
	if err != nil {
		t.Fatal(err)
	}
	if response.Summary.RequestCount != 1 || response.Summary.InputTokens != 12 || response.Summary.OutputTokens != 8 || response.Summary.TotalTokens != 20 {
		t.Fatalf("quiet logging interrupted analytics usage collection %#v", response.Summary)
	}
}
