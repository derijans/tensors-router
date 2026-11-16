package tufpublish

import "net/http"

const githubAPIHost = "api.github.com"

type githubAPIAuthorization struct {
	base  http.RoundTripper
	token string
}

func WithGitHubAPIAuthorization(base http.RoundTripper, token string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if token == "" {
		return base
	}
	return githubAPIAuthorization{base: base, token: token}
}

func (authorization githubAPIAuthorization) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != "https" || request.URL.Host != githubAPIHost {
		return authorization.base.RoundTrip(request)
	}
	authenticated := request.Clone(request.Context())
	authenticated.Header.Set("Authorization", "Bearer "+authorization.token)
	return authorization.base.RoundTrip(authenticated)
}
