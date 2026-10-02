package main

import (
	"strings"
	"testing"
)

func TestRunRejectsPositionalArgumentsInsteadOfIgnoringFlags(t *testing.T) {
	err := run([]string{"serve", "-config", "does-not-exist.yaml"})
	if err == nil || !strings.Contains(err.Error(), `unexpected argument "serve"`) {
		t.Fatalf("a stray subcommand silently dropped the following flags: %v", err)
	}
}
