package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tensors-router/internal/cluster"
	"tensors-router/internal/routinggroups"
)

const slaveOwnsCC11LinksBody = `{"image":[{"owner":{"node_id":"slave","model_id":"cc11-ff"},"helper":{"node_id":"master","model_id":"cc-ff"},"load_if_unloaded":true}],"text":[]}`

func postNodeRoutingLinks(service *Service, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, nodeRoutingLinksPath, strings.NewReader(slaveOwnsCC11LinksBody))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	service.ServeHTTP(recorder, request)
	return recorder
}

func TestSlaveInstallsLinksPushedByItsMaster(t *testing.T) {
	service := newLinkedImageService(t, nil, false)
	service.clusterRole = cluster.RoleSlave
	service.nodeID = "slave"

	if recorder := postNodeRoutingLinks(service, "secret"); recorder.Code != http.StatusNoContent {
		t.Fatalf("status %d body %s, want 204", recorder.Code, recorder.Body.String())
	}
	if !service.scheduler.queuesForLending(cluster.RouteLaneImage, "slave", "cc11-ff") {
		t.Fatal("the pushed owner link was not installed")
	}
	owners := service.routingLinkIndex().owners(cluster.RouteLaneImage)
	if len(owners) != 1 || owners[0] != (routinggroups.Endpoint{NodeID: "slave", ModelID: "cc11-ff"}) {
		t.Fatalf("owners = %+v, want only cc11-ff on the slave", owners)
	}
}

func TestRoutingLinkPushRequiresTheClusterToken(t *testing.T) {
	service := newLinkedImageService(t, nil, false)
	service.clusterRole = cluster.RoleSlave

	if recorder := postNodeRoutingLinks(service, "wrong"); recorder.Code == http.StatusNoContent {
		t.Fatal("links were accepted with a wrong cluster token")
	}
	if recorder := postNodeRoutingLinks(service, ""); recorder.Code == http.StatusNoContent {
		t.Fatal("links were accepted without a cluster token")
	}
	if service.scheduler.queuesForLending(cluster.RouteLaneImage, "slave", "cc11-ff") {
		t.Fatal("a rejected push still installed links")
	}
}

func TestOnlyASlaveAcceptsPushedLinks(t *testing.T) {
	service := newLinkedImageService(t, nil, false)
	service.clusterRole = cluster.RoleMaster

	if recorder := postNodeRoutingLinks(service, "secret"); recorder.Code != http.StatusConflict {
		t.Fatalf("status %d body %s, want 409 on a master", recorder.Code, recorder.Body.String())
	}
}

func TestLendTargetsFollowOnlyTheOwnersDirection(t *testing.T) {
	master := routinggroups.Endpoint{NodeID: "master", ModelID: "cc-ff"}
	slave := routinggroups.Endpoint{NodeID: "slave", ModelID: "cc11-ff"}
	index := newRoutingLinkIndex(routingLinkSnapshot{Image: []routinggroups.Link{{Owner: master, Helper: slave}}})

	if targets := index.lendTargets(cluster.RouteLaneImage, master); len(targets) != 1 || targets[0].Helper != slave {
		t.Fatalf("master targets = %+v, want the slave", targets)
	}
	if targets := index.lendTargets(cluster.RouteLaneImage, slave); len(targets) != 0 {
		t.Fatalf("slave targets = %+v, want none: the link only lends from the master", targets)
	}
	if !index.isLinked(cluster.RouteLaneImage, slave) {
		t.Fatal("a helper must queue too, so borrowed work it runs is tracked")
	}
	if index.isLinked(cluster.RouteLaneText, master) {
		t.Fatal("an image link leaked into the text lane")
	}
}
