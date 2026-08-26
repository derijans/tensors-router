package tufpublish

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubRepositoryPartsRequireCanonicalHTTPSURL(t *testing.T) {
	parts, err := githubRepositoryParts("https://github.com/owner/repository")
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 2 || parts[0] != "owner" || parts[1] != "repository" {
		t.Fatalf("unexpected repository parts %#v", parts)
	}
	for _, rawURL := range []string{
		"http://github.com/owner/repository",
		"https://token@github.com/owner/repository",
		"https://github.com/owner/repository?ref=main",
		"https://github.com/owner/repository#main",
		"https://github.com/owner/repository/extra",
		"https://example.com/owner/repository",
	} {
		if _, err := githubRepositoryParts(rawURL); err == nil {
			t.Fatalf("expected canonical repository rejection for %s", rawURL)
		}
	}
}

func TestHTTPSOnlyClientRejectsDowngradeAndExcessiveRedirects(t *testing.T) {
	client := httpsOnlyClient(&http.Client{})
	downgrade := &http.Request{URL: &url.URL{Scheme: "http", Host: "downloads.example.test"}}
	if err := client.CheckRedirect(downgrade, nil); err == nil {
		t.Fatal("expected HTTPS downgrade rejection")
	}
	secure := &http.Request{URL: &url.URL{Scheme: "https", Host: "downloads.example.test"}}
	via := make([]*http.Request, 10)
	if err := client.CheckRedirect(secure, via); err == nil {
		t.Fatal("expected redirect limit rejection")
	}
	if err := client.CheckRedirect(secure, via[:1]); err != nil {
		t.Fatal(err)
	}
}

func TestHashAssetRejectsNonHTTPSURLBeforeNetwork(t *testing.T) {
	if _, _, err := hashAsset(context.Background(), &http.Client{}, asset{Name: "payload", URL: "http://downloads.example.test/payload", Size: 1}); err == nil {
		t.Fatal("expected non-HTTPS asset rejection")
	}
}

func llamaSource() Source {
	return Source{
		Backend:    "llama-server",
		Repository: "https://github.com/ggml-org/llama.cpp",
		Platform:   "windows-amd64",
		AssetGlob:  "llama-*-bin-win-cpu-x64.zip",
	}
}

// llama.cpp publishes a stable marker release carrying only nightly-tag.txt
// next to its real per-build releases. It is the newest non-prerelease, so
// treating "no matching asset" as fatal broke every scheduled publication
// until the marker aged out.
func TestSelectReleaseAssetSkipsAReleaseCarryingNoMatchingAsset(t *testing.T) {
	releases := []release{
		{Tag: "v0.3.0", Assets: []asset{{Name: "nightly-tag.txt", URL: "https://example.test/nightly-tag.txt", Size: 12}}},
		{Tag: "b10636", Assets: []asset{
			{Name: "llama-b10636-bin-win-cpu-x64.zip", URL: "https://example.test/win.zip", Size: 1024},
			{Name: "llama-b10636-bin-ubuntu-x64.tar.gz", URL: "https://example.test/linux.tgz", Size: 2048},
		}},
	}

	selectedRelease, selectedAsset, err := selectReleaseAsset(releases, llamaSource())
	if err != nil {
		t.Fatalf("marker release must be skipped, not fatal: %v", err)
	}
	if selectedRelease.Tag != "b10636" {
		t.Fatalf("selected release %q, want b10636", selectedRelease.Tag)
	}
	if selectedAsset.Name != "llama-b10636-bin-win-cpu-x64.zip" {
		t.Fatalf("selected asset %q", selectedAsset.Name)
	}
}

// A glob loose enough to match two downloads cannot identify what to publish,
// so it must fail loudly rather than silently picking one.
func TestSelectReleaseAssetRejectsAnAmbiguousGlob(t *testing.T) {
	releases := []release{{Tag: "b10636", Assets: []asset{
		{Name: "llama-b10636-bin-win-cpu-x64.zip", URL: "https://example.test/a.zip", Size: 1024},
		{Name: "llama-b10636-extra-bin-win-cpu-x64.zip", URL: "https://example.test/b.zip", Size: 1024},
	}}}

	if _, _, err := selectReleaseAsset(releases, llamaSource()); err == nil {
		t.Fatal("expected an ambiguous glob to be rejected")
	}
}

func TestSelectReleaseAssetSkipsDraftsAndPrereleases(t *testing.T) {
	matching := []asset{{Name: "llama-x-bin-win-cpu-x64.zip", URL: "https://example.test/x.zip", Size: 1024}}
	releases := []release{
		{Tag: "draft", Draft: true, Assets: matching},
		{Tag: "pre", Prerelease: true, Assets: matching},
		{Tag: "stable", Assets: matching},
	}

	selectedRelease, _, err := selectReleaseAsset(releases, llamaSource())
	if err != nil {
		t.Fatal(err)
	}
	if selectedRelease.Tag != "stable" {
		t.Fatalf("selected %q, want stable", selectedRelease.Tag)
	}

	source := llamaSource()
	source.IncludePrereleases = true
	selectedRelease, _, err = selectReleaseAsset(releases, source)
	if err != nil {
		t.Fatal(err)
	}
	if selectedRelease.Tag != "pre" {
		t.Fatalf("with prereleases enabled selected %q, want pre", selectedRelease.Tag)
	}
}

// Skipping assetless releases must not become "publish nothing quietly": when
// no release carries the asset at all, that is still a hard failure.
func TestSelectReleaseAssetFailsWhenNoReleaseCarriesTheAsset(t *testing.T) {
	releases := []release{
		{Tag: "v0.3.0", Assets: []asset{{Name: "nightly-tag.txt", URL: "https://example.test/n.txt", Size: 12}}},
		{Tag: "v0.2.0", Assets: nil},
	}

	_, _, err := selectReleaseAsset(releases, llamaSource())
	if err == nil {
		t.Fatal("expected a failure when nothing carries the asset")
	}
	if !strings.Contains(err.Error(), "llama-*-bin-win-cpu-x64.zip") {
		t.Fatalf("error should name the unmatched glob, got %v", err)
	}
}

func TestSelectReleaseAssetRejectsAnEmptyAsset(t *testing.T) {
	releases := []release{{Tag: "b10636", Assets: []asset{
		{Name: "llama-b10636-bin-win-cpu-x64.zip", URL: "https://example.test/a.zip", Size: 0},
	}}}

	if _, _, err := selectReleaseAsset(releases, llamaSource()); err == nil {
		t.Fatal("expected a zero-size asset to be rejected")
	}
}

// llama.cpp marks every per-build release a prerelease and keeps a single
// stable marker release that carries no binaries, so llama-server can only be
// discovered with prereleases enabled. Every other upstream publishes real
// stable releases and must stay on the stable channel.
func TestUpstreamConfigEnablesPrereleasesOnlyWhereTheUpstreamRequiresThem(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "tuf", "upstreams.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config Config
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Sources) == 0 {
		t.Fatal("upstreams.json declares no sources")
	}
	for _, source := range config.Sources {
		wantPrereleases := source.Backend == "llama-server"
		if source.IncludePrereleases != wantPrereleases {
			t.Errorf("%s/%s include_prereleases = %t, want %t", source.Backend, source.Platform, source.IncludePrereleases, wantPrereleases)
		}
		if source.Backend == "" || source.Platform == "" || source.AssetGlob == "" {
			t.Errorf("%s/%s has an incomplete source definition", source.Backend, source.Platform)
		}
	}
}
