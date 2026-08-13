package main

import "testing"

func TestParseWorkerConfigRestrictsCommandSurface(t *testing.T) {
	configuration, err := parseWorkerConfig([]string{"--data-dir", "/data/vllm", "--profile", "auto", "--manifest", "/data/manifest.json", "--manifest-size", "42", "--manifest-sha256", "digest", "--allow-trust-remote-code", "TRUE"})
	if err != nil {
		t.Fatal(err)
	}
	if configuration.DataDir != "/data/vllm" || configuration.ManifestSize != 42 || !configuration.AllowTrustRemoteCode {
		t.Fatalf("unexpected configuration %#v", configuration)
	}
	if _, err := parseWorkerConfig([]string{"--data-dir", "/data", "--shell", "command"}); err == nil {
		t.Fatal("expected unknown worker option rejection")
	}
	if _, err := parseWorkerConfig([]string{"--allow-external-tools", "sometimes"}); err == nil {
		t.Fatal("expected invalid administrator boolean rejection")
	}
}
