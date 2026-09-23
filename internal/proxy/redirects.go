package proxy

import "net/http"

func returnRedirectToCaller(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}
