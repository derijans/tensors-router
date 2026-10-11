package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tensors-router/internal/guardianmodels"
	"tensors-router/internal/routerstore/routerstoretest"
	"tensors-router/internal/siteapi"
)

const guardianModelsPath = "/router/v1/site/guardian-models"

func guardianHarnessWithStore(t *testing.T) guardianHarness {
	t.Helper()
	harness := newGuardianHarness(t, false)
	handle := routerstoretest.Open(t, guardianmodels.SchemaModule{})
	harness.service.guardians = newGuardianDirectory(guardianmodels.NewStore(handle.DB(), handle.Reader()))
	return harness
}

func guardianRequest(harness guardianHarness, method string, target string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	harness.service.ServeHTTP(recorder, request)
	return recorder
}

func decodeGuardianModels(t *testing.T, recorder *httptest.ResponseRecorder) siteapi.GuardianModelsResponse {
	t.Helper()
	var response siteapi.GuardianModelsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body.String(), err)
	}
	return response
}

func TestGuardianAssignmentIsStoredAndActsAtOnce(t *testing.T) {
	harness := guardianHarnessWithStore(t)

	saved := guardianRequest(harness, http.MethodPost, guardianModelsPath, `{"scope":"cluster","guardian":{"node_id":"`+harness.service.nodeID+`","model_id":"vision"}}`)

	if saved.Code != http.StatusOK {
		t.Fatalf("save = %d %s", saved.Code, saved.Body.String())
	}
	if assignments := decodeGuardianModels(t, saved).Assignments; len(assignments) != 1 || assignments[0].Guardian.ModelID != "vision" {
		t.Fatalf("assignments = %+v, want the saved cluster guardian", assignments)
	}
	if answer := harness.chat(imageChatBody); !strings.Contains(answer.Body.String(), "answered by vision.kcpps") {
		t.Fatalf("response = %s, want the new guardian used immediately", answer.Body.String())
	}
}

func TestOnlyMultimodalTextModelsAreOfferedAndAccepted(t *testing.T) {
	harness := guardianHarnessWithStore(t)

	listed := decodeGuardianModels(t, guardianRequest(harness, http.MethodGet, guardianModelsPath, ""))
	if len(listed.Candidates) != 1 || listed.Candidates[0].ModelID != "vision" {
		t.Fatalf("candidates = %+v, want only the multimodal model", listed.Candidates)
	}
	rejected := guardianRequest(harness, http.MethodPost, guardianModelsPath, `{"scope":"cluster","guardian":{"node_id":"`+harness.service.nodeID+`","model_id":"plain"}}`)
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("saving a text-only guardian = %d, want 400", rejected.Code)
	}
	unknown := guardianRequest(harness, http.MethodPost, guardianModelsPath, `{"scope":"cluster","guardian":{"node_id":"nowhere","model_id":"vision"}}`)
	if unknown.Code != http.StatusBadRequest {
		t.Fatalf("saving an unknown guardian = %d, want 400", unknown.Code)
	}
}

func TestClearingAGuardianStopsItActing(t *testing.T) {
	harness := guardianHarnessWithStore(t)
	guardianRequest(harness, http.MethodPost, guardianModelsPath, `{"scope":"cluster","guardian":{"node_id":"`+harness.service.nodeID+`","model_id":"vision"}}`)

	cleared := guardianRequest(harness, http.MethodDelete, guardianModelsPath+"?scope=cluster", "")

	if cleared.Code != http.StatusOK || len(decodeGuardianModels(t, cleared).Assignments) != 0 {
		t.Fatalf("clear = %d %s, want no assignments left", cleared.Code, cleared.Body.String())
	}
	if answer := harness.chat(imageChatBody); answer.Code != http.StatusBadRequest {
		t.Fatalf("response = %d, want the plain model's refusal once the guardian is gone", answer.Code)
	}
}
