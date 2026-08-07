package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentLoadOptionsReadsCredentialFiles(t *testing.T) {
	dir := t.TempDir()
	inferencePath := filepath.Join(dir, "inference")
	adminPath := filepath.Join(dir, "admin")
	clusterPath := filepath.Join(dir, "cluster")
	for path, value := range map[string]string{
		inferencePath: "inference-secret",
		adminPath:     "admin-secret",
		clusterPath:   "cluster-secret",
	} {
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(inferenceTokenFileEnvironment, inferencePath)
	t.Setenv(adminTokenFileEnvironment, adminPath)
	t.Setenv(clusterTokenFileEnvironment, clusterPath)

	options, err := environmentLoadOptions("secure")
	if err != nil {
		t.Fatal(err)
	}
	if options.InferenceKey != "inference-secret" || options.AdminKey != "admin-secret" || options.ClusterToken != "cluster-secret" {
		t.Fatalf("unexpected credential options %#v", options)
	}
}

func TestEnvironmentLoadOptionsRejectsValueAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin")
	if err := os.WriteFile(path, []byte("file-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(adminTokenEnvironment, "value-secret")
	t.Setenv(adminTokenFileEnvironment, path)

	_, err := environmentLoadOptions("")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected ambiguity rejection, got %v", err)
	}
}
