package proxy

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"tensors-router/internal/credentialstore"
)

func newNodeProfileTestRoutes(t *testing.T, spy *profileFileSpy) (*routeTable, *credentialstore.Live) {
	t.Helper()
	live := openTestCredentials(t)
	admin := newSecurityAdmin(SecurityConfig{Credentials: live, Profile: spy.control()}, "worker")
	acceptAnyClusterCaller := func(http.ResponseWriter, *http.Request) bool { return true }
	return newRouteTable(acceptAnyClusterCaller, admin.nodeRoutes(), admin.routes()), live
}

func storeClusterToken(t *testing.T, live *credentialstore.Live, secret string) {
	t.Helper()
	ctx := context.Background()
	if _, _, err := live.Store().EnsureClusterToken(ctx, "cluster", secret); err != nil {
		t.Fatal(err)
	}
	if err := live.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
}

func switchNodeProfile(routes *routeTable, bearer string) (int, securityProfileStatus, string) {
	response := serveSecurityRequest(http.HandlerFunc(routes.serve), http.MethodPost, securityNodeProfilePath, "{}", bearer)
	var status securityProfileStatus
	_ = strictJSONUnmarshal(response.Body.Bytes(), &status)
	return response.Code, status, response.Body.String()
}

func TestNodeProfileSwitchRequiresTheClusterTokenTheMasterKeepsUsing(t *testing.T) {
	spy := &profileFileSpy{}
	routes, live := newNodeProfileTestRoutes(t, spy)
	storeClusterToken(t, live, securityTestClusterToken)
	adminSecret := createAdminToken(t, routes)

	for _, bearer := range []string{"", adminSecret, "not-a-cluster-token"} {
		if code, _, body := switchNodeProfile(routes, bearer); code != http.StatusConflict || spy.writes != 0 {
			t.Fatalf("switch with bearer %q: status %d writes %d: %s", bearer, code, spy.writes, body)
		}
	}
	code, status, body := switchNodeProfile(routes, securityTestClusterToken)
	if code != http.StatusOK || spy.writes != 1 || !status.ProfileRestartRequired {
		t.Fatalf("switch with cluster token: status %d writes %d: %s", code, spy.writes, body)
	}
	if code, _, _ := switchNodeProfile(routes, securityTestClusterToken); code != http.StatusOK || spy.writes != 1 {
		t.Fatalf("repeated switch: status %d writes %d", code, spy.writes)
	}
}

func TestNodeProfileSwitchKeepsTheSecureProfileBlocker(t *testing.T) {
	spy := &profileFileSpy{blocker: errors.New("an inference token is required")}
	routes, live := newNodeProfileTestRoutes(t, spy)
	storeClusterToken(t, live, securityTestClusterToken)
	if code, _, body := switchNodeProfile(routes, securityTestClusterToken); code != http.StatusConflict || spy.writes != 0 {
		t.Fatalf("switch while blocked: status %d writes %d: %s", code, spy.writes, body)
	}
}

func TestMasterRelaysTheSecureProfileSwitchToTheSlave(t *testing.T) {
	spy := &profileFileSpy{}
	testCluster := newSecurityTestClusterWithSlaveProfile(t, spy.control())
	storeClusterToken(t, testCluster.slaveLive, securityTestClusterToken)

	response := serveSecurityRequest(testCluster.master, http.MethodPost, masterRelayPath("profile"), "{}", "")
	if response.Code != http.StatusOK || spy.writes != 1 {
		t.Fatalf("relayed switch: status %d writes %d: %s", response.Code, spy.writes, response.Body)
	}
	if status := decodeRecorded[securityProfileStatus](t, response.Body.Bytes()); status.Profile != "trusted_lan" || !status.ProfileRestartRequired {
		t.Fatalf("relayed status %#v", status)
	}
}
