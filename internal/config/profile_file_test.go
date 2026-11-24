package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithSecurityProfileReplacesOnlyTheSecurityProfileValue(t *testing.T) {
	content := "security:\n  profile: \"trusted_lan\" # lab only\nvllm:\n  profile: \"trusted_lan\"\n"
	got := withSecurityProfile(content, SecurityProfileSecure)
	want := "security:\n  profile: \"secure\" # lab only\nvllm:\n  profile: \"trusted_lan\"\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWithSecurityProfileKeepsWindowsLineEndings(t *testing.T) {
	got := withSecurityProfile("server:\r\n  bind: \"127.0.0.1:8080\"\r\nsecurity:\r\n  profile: trusted_lan\r\n", SecurityProfileSecure)
	want := "server:\r\n  bind: \"127.0.0.1:8080\"\r\nsecurity:\r\n  profile: \"secure\"\r\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWithSecurityProfileAddsMissingKeyAndSection(t *testing.T) {
	withoutKey := withSecurityProfile("security:\nserver:\n  bind: \"127.0.0.1:8080\"\n", SecurityProfileSecure)
	if withoutKey != "security:\n  profile: \"secure\"\nserver:\n  bind: \"127.0.0.1:8080\"\n" {
		t.Fatalf("missing key not added: %q", withoutKey)
	}
	withoutSection := withSecurityProfile("server:\n  bind: \"127.0.0.1:8080\"\n", SecurityProfileSecure)
	if !strings.HasPrefix(withoutSection, "security:\n  profile: \"secure\"\nserver:") {
		t.Fatalf("missing section not added: %q", withoutSection)
	}
}

func TestWriteSecurityProfileProducesLoadableSecureConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("security:\n  profile: \"trusted_lan\"\nserver:\n  bind: \"127.0.0.1:8080\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteSecurityProfile(path, SecurityProfileSecure); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.Profile != SecurityProfileSecure {
		t.Fatalf("profile = %q, want secure", cfg.Security.Profile)
	}
}

func TestSecureProfileBlockerRequiresTokensForNonLoopbackBind(t *testing.T) {
	cfg, err := loadConfigText(t, "security:\n  profile: \"trusted_lan\"\nserver:\n  bind: \"0.0.0.0:8080\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SecureProfileBlocker(StoredCredentials{Admin: true}); err == nil {
		t.Fatal("expected a missing inference token to block the secure profile")
	}
	if err := cfg.SecureProfileBlocker(StoredCredentials{Inference: true, Admin: true}); err != nil {
		t.Fatalf("unexpected blocker: %v", err)
	}
}
