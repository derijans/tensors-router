package tufpublish

import (
	"context"
	"net/http"
	"net/url"
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
