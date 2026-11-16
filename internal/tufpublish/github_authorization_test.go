package tufpublish

import (
	"net/http"
	"testing"
)

type recordingTransport struct {
	authorizations map[string]string
}

func (transport *recordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.authorizations[request.URL.String()] = request.Header.Get("Authorization")
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: request}, nil
}

func sendThrough(t *testing.T, transport http.RoundTripper, rawURL string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(request); err != nil {
		t.Fatal(err)
	}
	return request
}

func TestGitHubAPIAuthorizationAuthenticatesOnlyTheHTTPSGitHubAPI(t *testing.T) {
	recorder := &recordingTransport{authorizations: map[string]string{}}
	transport := WithGitHubAPIAuthorization(recorder, "secret")

	apiURL := "https://api.github.com/repos/owner/repository/releases"
	original := sendThrough(t, transport, apiURL)
	if got := recorder.authorizations[apiURL]; got != "Bearer secret" {
		t.Fatalf("GitHub API authorization = %q", got)
	}
	if original.Header.Get("Authorization") != "" {
		t.Fatal("caller request must not be mutated")
	}

	for _, rawURL := range []string{
		"https://github.com/owner/repository/releases/download/v1/asset.zip",
		"https://release-assets.githubusercontent.com/asset.zip",
		"http://api.github.com/repos/owner/repository/releases",
	} {
		sendThrough(t, transport, rawURL)
		if got := recorder.authorizations[rawURL]; got != "" {
			t.Fatalf("%s received authorization %q", rawURL, got)
		}
	}
}

func TestGitHubAPIAuthorizationWithoutTokenLeavesTransportUnchanged(t *testing.T) {
	recorder := &recordingTransport{authorizations: map[string]string{}}
	if WithGitHubAPIAuthorization(recorder, "") != http.RoundTripper(recorder) {
		t.Fatal("empty token must return the base transport")
	}
}
