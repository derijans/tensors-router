package auth

import (
	"testing"

	"tensors-router/internal/credential"
)

func TestPrincipalForSeparatesAdminAndClusterCallers(t *testing.T) {
	keys := credential.KeyHashes{
		Admin:   []credential.TokenHash{credential.HashToken("admin-secret")},
		Cluster: []credential.TokenHash{credential.HashToken("cluster-secret")},
	}
	cases := map[string]Principal{
		"Bearer admin-secret":   {Admin: true},
		"Bearer cluster-secret": {Cluster: true},
		"Bearer other":          {},
		"":                      {},
	}
	for authorization, want := range cases {
		if got := PrincipalFor(authorization, keys); got != want {
			t.Fatalf("PrincipalFor(%q) = %#v, want %#v", authorization, got, want)
		}
	}
}
