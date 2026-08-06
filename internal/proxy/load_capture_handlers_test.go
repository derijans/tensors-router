package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"tensors-router/internal/loadcapture"
)

func TestSiteLoadCapturesListsDetailsAndRejectsUnknownNodes(t *testing.T) {
	store, err := loadcapture.NewStore(loadcapture.StoreConfig{NodeID: "local", DatabasePath: filepath.Join(t.TempDir(), "captures.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot := loadcapture.Snapshot{SHA256: strings.Repeat("a", 64), JSON: []byte(`{"threads":8}`)}
	attempt, err := store.BeginPhysical(context.Background(), snapshot, "kobold", "koboldcpp", "llm")
	if err != nil {
		t.Fatal(err)
	}
	capture := loadcapture.Capture{CapturedBytes: 5, Chunks: []loadcapture.Chunk{{Sequence: 1, Stream: loadcapture.StreamStdout, Payload: []byte("ready")}}}
	if err := store.CompletePhysical(context.Background(), attempt, nil, capture, nil); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{NodeID: "local", LoadCaptureStore: store})

	listRecorder := httptest.NewRecorder()
	service.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/load-captures?limit=10&status=succeeded", nil))
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("list status %d: %s", listRecorder.Code, listRecorder.Body.String())
	}
	var list loadCaptureListResponse
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if !list.Enabled || len(list.Nodes) != 1 || len(list.Attempts) != 1 || list.Attempts[0].ID != attempt.ID {
		t.Fatalf("unexpected list response: %#v", list)
	}

	detailRecorder := httptest.NewRecorder()
	service.ServeHTTP(detailRecorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/load-captures/"+attempt.ID+"?node_id=local", nil))
	if detailRecorder.Code != http.StatusOK || !strings.Contains(detailRecorder.Body.String(), `"threads":8`) {
		t.Fatalf("unexpected detail response %d: %s", detailRecorder.Code, detailRecorder.Body.String())
	}

	outputRecorder := httptest.NewRecorder()
	service.ServeHTTP(outputRecorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/load-captures/"+attempt.ID+"/output?node_id=local", nil))
	if outputRecorder.Code != http.StatusOK || !strings.Contains(outputRecorder.Body.String(), `"payload":"cmVhZHk="`) {
		t.Fatalf("unexpected output response %d: %s", outputRecorder.Code, outputRecorder.Body.String())
	}

	invalidRecorder := httptest.NewRecorder()
	service.ServeHTTP(invalidRecorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/load-captures?node_id=unknown", nil))
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid node rejection, got %d", invalidRecorder.Code)
	}

	missingOutputRecorder := httptest.NewRecorder()
	service.ServeHTTP(missingOutputRecorder, httptest.NewRequest(http.MethodGet, "/router/v1/site/load-captures/missing/output?node_id=local", nil))
	if missingOutputRecorder.Code != http.StatusNotFound {
		t.Fatalf("expected missing output rejection, got %d", missingOutputRecorder.Code)
	}
}
