package proxy

import (
	"errors"
	"net/http"

	"tensors-router/internal/auth"
	"tensors-router/internal/credential"
)

const securityNodeProfilePath = securityNodePath + "/profile"

var errSecureProfileLocksOutMaster = errors.New("this request carries no cluster token this node accepts, so the master would lose access to it after the switch")

func (admin *securityAdmin) handleNodeSwitchToSecureProfile(w http.ResponseWriter, r *http.Request) {
	admin.switchToSecureProfile(w, r, clusterCallerKeepsAccess)
}

func clusterCallerKeepsAccess(r *http.Request, keys credential.KeyHashes) error {
	if !auth.PrincipalFor(r.Header.Get("Authorization"), keys).Cluster {
		return errSecureProfileLocksOutMaster
	}
	return nil
}
