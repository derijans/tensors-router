package vllm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type stubManifestSource struct {
	manifest Manifest
	digest   string
	err      error
	trust    ManifestTrust
	calls    *int
}

func (source stubManifestSource) Load(context.Context) (Manifest, string, error) {
	if source.calls != nil {
		*source.calls++
	}
	if source.err != nil {
		return Manifest{}, "", source.err
	}
	return source.manifest, source.digest, nil
}

func (source stubManifestSource) ManifestTrust() ManifestTrust {
	return source.trust
}

func TestFallbackManifestSourceFallsBackOnlyForUnpublishedManifests(t *testing.T) {
	fallbackManifest := testManifest()
	fallbackManifest.Release = "9.9.9"
	fallbackCalls := 0
	fallback := stubManifestSource{manifest: fallbackManifest, digest: strings.Repeat("b", 64), trust: ManifestTrustEmbeddedDefault, calls: &fallbackCalls}

	source := FallbackManifestSource{
		Primary:  stubManifestSource{err: fmt.Errorf("resolve trusted vLLM manifest: %w", ErrManifestNotPublished), trust: ManifestTrustTUF},
		Fallback: fallback,
	}
	manifest, digest, trust, err := source.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Release != "9.9.9" || digest != strings.Repeat("b", 64) {
		t.Fatalf("unexpected fallback manifest %q %q", manifest.Release, digest)
	}
	if trust != ManifestTrustEmbeddedDefault {
		t.Fatalf("unexpected trust tier %q", trust)
	}
	if fallbackCalls != 1 {
		t.Fatalf("fallback consulted %d times", fallbackCalls)
	}
}

func TestFallbackManifestSourceFailsClosedOnEveryOtherError(t *testing.T) {
	fallbackCalls := 0
	fallback := stubManifestSource{manifest: testManifest(), digest: strings.Repeat("b", 64), trust: ManifestTrustEmbeddedDefault, calls: &fallbackCalls}
	for _, primaryError := range []error{
		fmt.Errorf("refresh vLLM trusted metadata: expired timestamp"),
		fmt.Errorf("download trusted vLLM manifest: unexpected EOF"),
		fmt.Errorf("trusted vLLM manifest lacks SHA-256 metadata"),
		fmt.Errorf("vLLM manifest SHA-256 does not match TUF target metadata"),
	} {
		source := FallbackManifestSource{
			Primary:  stubManifestSource{err: primaryError, trust: ManifestTrustTUF},
			Fallback: fallback,
		}
		if _, _, _, err := source.Resolve(context.Background()); err == nil {
			t.Fatalf("fallback masked %v", primaryError)
		}
	}
	if fallbackCalls != 0 {
		t.Fatalf("fallback consulted %d times for non-recoverable failures", fallbackCalls)
	}
}

func TestFallbackManifestSourceReportsBothFailuresWhenNoDefaultExists(t *testing.T) {
	source := FallbackManifestSource{
		Primary:  stubManifestSource{err: fmt.Errorf("resolve trusted vLLM manifest: %w", ErrManifestNotPublished), trust: ManifestTrustTUF},
		Fallback: EmbeddedManifestSource{Platform: "plan9-vax"},
	}
	_, _, _, err := source.Resolve(context.Background())
	if err == nil {
		t.Fatal("a missing embedded default was accepted")
	}
	if !errors.Is(err, ErrManifestNotPublished) || !strings.Contains(err.Error(), "no fallback manifest is available") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestResolveManifestReportsStaticTrustTiers(t *testing.T) {
	for _, test := range []struct {
		source ManifestSource
		trust  ManifestTrust
	}{
		{TUFManifestSource{}, ManifestTrustTUF},
		{AuthorizedManifestFile{}, ManifestTrustOperatorPinned},
		{EmbeddedManifestSource{}, ManifestTrustEmbeddedDefault},
	} {
		if got := staticManifestTrust(test.source); got != test.trust {
			t.Fatalf("%T reported trust %q", test.source, got)
		}
	}
}

func TestEmbeddedDefaultManifestsAreValid(t *testing.T) {
	for _, platform := range []string{"linux-amd64", "linux-arm64", "darwin-arm64"} {
		content, found := EmbeddedManifest(platform)
		if !found {
			continue
		}
		manifest, err := ParseManifest(content)
		if err != nil {
			t.Fatalf("embedded default %s: %v", platform, err)
		}
		var reencoded Manifest
		if err := json.Unmarshal(content, &reencoded); err != nil {
			t.Fatalf("embedded default %s is not valid JSON: %v", platform, err)
		}
		if len(manifest.Profiles) == 0 {
			t.Fatalf("embedded default %s declares no profiles", platform)
		}
	}
}

func TestEmbeddedManifestRejectsUnsafePlatformNames(t *testing.T) {
	for _, platform := range []string{"", "../defaults/linux-amd64", "linux-amd64/../../go.mod"} {
		if _, found := EmbeddedManifest(platform); found {
			t.Fatalf("unsafe platform %q resolved an embedded manifest", platform)
		}
	}
}
