package cluster

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

type Credentials interface {
	OutboundClusterToken() string
	AcceptsClusterToken(token string) bool
}

type StaticCredentials string

func (token StaticCredentials) OutboundClusterToken() string {
	return strings.TrimSpace(string(token))
}

func (token StaticCredentials) AcceptsClusterToken(presented string) bool {
	expected := token.OutboundClusterToken()
	return expected != "" && subtle.ConstantTimeCompare([]byte(strings.TrimSpace(presented)), []byte(expected)) == 1
}

func (client *Client) setAuthorization(request *http.Request) {
	if token := client.credentials.OutboundClusterToken(); token != "" {
		request.Header.Set("Authorization", bearerPrefix+token)
	}
}
