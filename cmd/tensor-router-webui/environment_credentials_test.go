package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebUICredentialOverridesReadFiles(t *testing.T) {
	dir := t.TempDir()
	adminPath := filepath.Join(dir, "webui-admin")
	routerPath := filepath.Join(dir, "managed-router")
	if err := os.WriteFile(adminPath, []byte("webui-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(routerPath, []byte("router-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(webUIAdminTokenFileEnvironment, adminPath)
	t.Setenv(managedRouterTokenFileEnvironment, routerPath)

	adminToken, routerToken, err := webUICredentialOverrides("", "")
	if err != nil {
		t.Fatal(err)
	}
	if adminToken != "webui-secret" || routerToken != "router-secret" {
		t.Fatalf("unexpected credentials admin=%q router=%q", adminToken, routerToken)
	}
}

func TestWebUICredentialOverridesRejectFileWithDirectValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "webui-admin")
	if err := os.WriteFile(path, []byte("file-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(webUIAdminTokenEnvironment, "value-secret")
	t.Setenv(webUIAdminTokenFileEnvironment, path)

	_, _, err := webUICredentialOverrides("", "")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected ambiguity rejection, got %v", err)
	}
}

func TestWebUICredentialOverridesPreferCommandLineValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "webui-admin")
	if err := os.WriteFile(path, []byte("file-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(webUIAdminTokenFileEnvironment, path)
	t.Setenv(managedRouterTokenEnvironment, "environment-router")

	adminToken, routerToken, err := webUICredentialOverrides("cli-admin", "cli-router")
	if err != nil {
		t.Fatal(err)
	}
	if adminToken != "cli-admin" || routerToken != "cli-router" {
		t.Fatalf("unexpected credentials admin=%q router=%q", adminToken, routerToken)
	}
}

func TestWebUICredentialOverridesIgnoreBlankCommandLineValues(t *testing.T) {
	t.Setenv(webUIAdminTokenEnvironment, "environment-admin")
	t.Setenv(managedRouterTokenEnvironment, "environment-router")

	adminToken, routerToken, err := webUICredentialOverrides("  ", "\t")
	if err != nil {
		t.Fatal(err)
	}
	if adminToken != "environment-admin" || routerToken != "environment-router" {
		t.Fatalf("unexpected credentials admin=%q router=%q", adminToken, routerToken)
	}
}
