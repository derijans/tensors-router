package main

import "testing"

func TestParseProfileCheckConfigRejectsIncompleteAndUnknownOptions(t *testing.T) {
	if _, err := parseProfileCheckConfig([]string{"--data-dir", "data"}); err == nil {
		t.Fatal("expected incomplete profile check to fail")
	}
	if _, err := parseProfileCheckConfig([]string{"--unknown", "value"}); err == nil {
		t.Fatal("expected unknown profile check option to fail")
	}
}

func TestParseProfileCheckConfig(t *testing.T) {
	configuration, err := parseProfileCheckConfig([]string{
		"--data-dir", "data",
		"--manifest", "manifest.json",
		"--profile", "cuda",
		"--expected-os", "linux",
		"--expected-architecture", "amd64",
		"--expected-device", "cuda",
	})
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Profile != "cuda" || configuration.ExpectedDevice != "cuda" {
		t.Fatalf("unexpected profile check configuration: %+v", configuration)
	}
}
