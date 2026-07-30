package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReconcileWritesSortedServersAndKoboldOverlay(t *testing.T) {
	configDir := t.TempDir()
	filename := "example.kcpps"
	writeConfig(t, configDir, filename, `{
  "backend_mode": "kobold",
  "mcp_enabled": true,
  "mcp_servers": [
    {"name":"zeta","definition":{"command":"z","args":["--z"],"env":{"TOKEN":"secret"}}},
    {"name":"alpha","definition":{"url":"https://example.invalid/mcp","headers":{"Authorization":"Bearer secret"}}}
  ]
}`)
	reconciler, err := NewReconciler(Config{Enabled: true, Directory: filepath.Join(configDir, "artifacts"), ConfigDir: configDir})
	if err != nil {
		t.Fatal(err)
	}
	result, err := reconciler.Reconcile(filename, BackendKobold)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Enabled || filepath.Base(filepath.Dir(result.ServersPath)) != "example" {
		t.Fatalf("unexpected result: %#v", result)
	}
	content, err := os.ReadFile(result.ServersPath)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"mcpServers":{"alpha":{"headers":{"Authorization":"Bearer secret"},"url":"https://example.invalid/mcp"},"zeta":{"args":["--z"],"command":"z","env":{"TOKEN":"secret"}}}}`
	if string(content) != expected {
		t.Fatalf("generated content = %s", content)
	}
	overlay, err := os.ReadFile(result.OverlayPath)
	if err != nil {
		t.Fatal(err)
	}
	var overlayConfig map[string]string
	if err := json.Unmarshal(overlay, &overlayConfig); err != nil || overlayConfig["mcpfile"] != result.ServersPath {
		t.Fatalf("unexpected overlay: %s", overlay)
	}
}

func TestReconcileRemovesDisabledArtifacts(t *testing.T) {
	configDir := t.TempDir()
	filename := "disabled.kcpps"
	writeConfig(t, configDir, filename, `{"mcp_enabled":true,"mcp_servers":[{"name":"server","definition":{"command":"server"}}]}`)
	reconciler, err := NewReconciler(Config{Enabled: true, Directory: filepath.Join(configDir, "artifacts"), ConfigDir: configDir})
	if err != nil {
		t.Fatal(err)
	}
	result, err := reconciler.Reconcile(filename, BackendLlama)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(result.ServersPath); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, configDir, filename, `{"mcp_enabled":false,"mcp_servers":[{"name":"server","definition":{"command":"server"}}]}`)
	if _, err := reconciler.Reconcile(filename, BackendLlama); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(result.ServersPath); !os.IsNotExist(err) {
		t.Fatalf("servers artifact still exists: %v", err)
	}
}

func TestReconcileAllSkipsLegacyValidationWhenGloballyDisabled(t *testing.T) {
	configDir := t.TempDir()
	filename := "legacy.kcpps"
	artifactDirectory := filepath.Join(configDir, "artifacts")
	writeConfig(t, configDir, filename, `{"mcp_enabled":true,"mcp_servers":[{"name":"server","definition":{"command":"server"}}]}`)
	enabledReconciler, err := NewReconciler(Config{Enabled: true, Directory: artifactDirectory, ConfigDir: configDir})
	if err != nil {
		t.Fatal(err)
	}
	result, err := enabledReconciler.Reconcile(filename, BackendKobold)
	if err != nil {
		t.Fatal(err)
	}
	writeConfig(t, configDir, filename, `{"mcpfile":"legacy.json"}`)
	disabledReconciler, err := NewReconciler(Config{Enabled: false, Directory: artifactDirectory, ConfigDir: configDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := disabledReconciler.ReconcileAll(BackendKobold); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{result.ServersPath, result.OverlayPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("disabled MCP artifact still exists at %q: %v", path, err)
		}
	}
}

func TestValidateRejectsDuplicateKeysAndUnsupportedTransport(t *testing.T) {
	if err := Validate([]byte(`{"mcp_servers":[],"mcp_servers":[]}`), BackendLlama); err == nil {
		t.Fatal("duplicate JSON keys were accepted")
	}
	if err := Validate([]byte(`{"mcp_servers":[{"name":"http","definition":{"url":"https://example.invalid"}}]}`), BackendLlama); err == nil {
		t.Fatal("llama HTTP transport was accepted")
	}
	if err := Validate([]byte(`{"mcpfile":"legacy.json"}`), BackendKobold); err == nil {
		t.Fatal("legacy MCP path was accepted")
	}
}

func writeConfig(t *testing.T, dir string, filename string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
