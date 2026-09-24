package main

import "testing"

func TestParseGenerateCheckConfigRejectsIncompleteAndUnknownOptions(t *testing.T) {
	if _, err := parseGenerateCheckConfig([]string{"--data-dir", "data"}); err == nil {
		t.Fatal("expected incomplete generate-check to fail")
	}
	if _, err := parseGenerateCheckConfig([]string{"--unknown", "value"}); err == nil {
		t.Fatal("expected unknown generate-check option to fail")
	}
	if _, err := parseGenerateCheckConfig([]string{"--data-dir", "data", "--expected-device", "cpu", "--max-tokens", "0"}); err == nil {
		t.Fatal("expected non-positive --max-tokens to fail")
	}
	if _, err := parseGenerateCheckConfig([]string{"--data-dir", "data", "--expected-device", "cpu", "--install-only", "maybe"}); err == nil {
		t.Fatal("expected non-boolean --install-only to fail")
	}
	if _, err := parseGenerateCheckConfig([]string{"--data-dir", "data", "--expected-device", "cpu"}); err == nil {
		t.Fatal("expected missing --model to fail without --install-only")
	}
}

func TestParseGenerateCheckConfig(t *testing.T) {
	configuration, err := parseGenerateCheckConfig([]string{
		"--data-dir", "data",
		"--model", "models/qwen",
		"--expected-device", "cpu",
		"--max-tokens", "8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if configuration.DataDir != "data" || configuration.ModelPath != "models/qwen" || configuration.ExpectedDevice != "cpu" || configuration.MaxTokens != 8 {
		t.Fatalf("unexpected generate-check configuration: %+v", configuration)
	}
	if configuration.Prompt == "" {
		t.Fatal("expected a default prompt")
	}
}

func TestParseGenerateCheckConfigInstallOnlyAllowsNoModel(t *testing.T) {
	configuration, err := parseGenerateCheckConfig([]string{
		"--data-dir", "data",
		"--expected-device", "rocm",
		"--install-only", "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !configuration.InstallOnly || configuration.ModelPath != "" {
		t.Fatalf("unexpected generate-check configuration: %+v", configuration)
	}
}
