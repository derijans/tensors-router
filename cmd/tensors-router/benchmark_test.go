package main

import (
	"testing"

	routerbenchmark "tensors-router/internal/benchmark"
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
