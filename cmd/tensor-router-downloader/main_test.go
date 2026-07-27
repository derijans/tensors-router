package main

import (
	"reflect"
	"testing"
)

func TestParseDownloadCommandAcceptsFlagsAfterRepository(t *testing.T) {
	command, err := parseDownloadCommand([]string{"owner/model", "model.gguf", "--revision", "v1", "--config", "node.yaml", "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if command.repository != "owner/model" || command.revision != "v1" || command.configPath != "node.yaml" || !command.yes || !reflect.DeepEqual(command.files, []string{"model.gguf"}) {
		t.Fatalf("unexpected command %#v", command)
	}
}

func TestParseDownloadCommandRestrictsSurface(t *testing.T) {
	command, err := parseDownloadCommand([]string{"owner/model", "--all", "--revision", "main"})
	if err != nil || command.mode != "snapshot" {
		t.Fatalf("unexpected snapshot command %#v err=%v", command, err)
	}
	if _, err := parseDownloadCommand([]string{"owner/model", "--all", "model.gguf"}); err == nil {
		t.Fatal("expected all/file conflict")
	}
	if _, err := parseDownloadCommand([]string{"owner/model", "model.gguf", "--worker"}); err == nil {
		t.Fatal("expected internal worker command rejection")
	}
}
