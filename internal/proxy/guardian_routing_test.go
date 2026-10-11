package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/guardianmodels"
	"tensors-router/internal/offloadsettings"
)

const (
	imageChatBody = `{"model":"plain","messages":[{"role":"user","content":[{"type":"text","text":"what is this"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}]}`
	textChatBody  = `{"model":"plain","messages":[{"role":"user","content":"hello"}]}`

	noVisionRefusal = `{"error":{"code":400,"message":"image input is not supported - hint: if this is unexpected, you may need to provide the mmproj","type":"invalid_request_error"}}`
)

type guardianHarness struct {
	service *Service
	backend *fakeBackend
	loaded  *atomic.Value
	refusal *atomic.Int32
	images  *atomic.Int32
}

func newGuardianHarness(t *testing.T, plainClaimsVision bool) guardianHarness {
	t.Helper()
	loaded := &atomic.Value{}
	loaded.Store("")
	refusal := &atomic.Int32{}
	refusal.Store(http.StatusBadRequest)
	images := &atomic.Int32{}
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(strings.Builder)
		_, _ = io.Copy(body, r.Body)
		w.Header().Set("Content-Type", "application/json")
		if loaded.Load() == "plain.kcpps" && strings.Contains(body.String(), "image_url") {
			images.Add(1)
			w.WriteHeader(int(refusal.Load()))
			_, _ = w.Write([]byte(noVisionRefusal))
			return
		}
		_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"answered by ` + loaded.Load().(string) + `"}}]}`))
	}), map[string]string{
		"plain":  `{"model_param":"C:\\models\\plain.gguf"}`,
		"vision": `{"model_param":"C:\\models\\vision.gguf","mmproj":"C:\\models\\mmproj.gguf"}`,
	})
	backend.onReload = func(filename string) { loaded.Store(filename) }
	tuneScheduler(service.scheduler, func(settings *offloadsettings.Settings) { settings.RestoreDelay = 20 * time.Millisecond })

	registry := cluster.NewRegistry(cluster.RoleStandalone, service.nodeID, "http://local")
	plain := testClusterModel("plain", service.nodeID, "plain-weights", "config-a", cluster.SourceLocal)
	plain.HasMultimodal = plainClaimsVision
	vision := testClusterModel("vision", service.nodeID, "vision-weights", "config-b", cluster.SourceLocal)
	vision.HasMultimodal = true
	if err := registry.UpdateLocal([]cluster.Model{plain, vision}); err != nil {
		t.Fatal(err)
	}
	service.registry = registry
	return guardianHarness{service: service, backend: backend, loaded: loaded, refusal: refusal, images: images}
}

func (harness guardianHarness) assign(assignment guardianmodels.Assignment) {
	harness.service.guardians.install([]guardianmodels.Assignment{assignment})
}

func (harness guardianHarness) clusterGuardian() guardianmodels.Assignment {
	return guardianmodels.Assignment{Scope: guardianmodels.ClusterScope, Guardian: guardianmodels.Endpoint{NodeID: harness.service.nodeID, ModelID: "vision"}}
}

func (harness guardianHarness) chat(body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	harness.service.ServeHTTP(recorder, request)
	return recorder
}

func (harness guardianHarness) waitForLoaded(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if harness.loaded.Load() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("loaded config = %v, want %s", harness.loaded.Load(), want)
}

func TestImageRequestForATextOnlyModelIsAnsweredByTheGuardian(t *testing.T) {
	harness := newGuardianHarness(t, false)
	harness.assign(harness.clusterGuardian())

	recorder := harness.chat(imageChatBody)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "answered by vision.kcpps") {
		t.Fatalf("response = %d %s, want the guardian's answer", recorder.Code, recorder.Body.String())
	}
}

func TestTheTextOnlyModelIsLoadedBackAfterTheGuardianAnswered(t *testing.T) {
	harness := newGuardianHarness(t, false)
	harness.assign(harness.clusterGuardian())
	if recorder := harness.chat(textChatBody); recorder.Code != http.StatusOK {
		t.Fatalf("warm-up response = %d %s", recorder.Code, recorder.Body.String())
	}

	if recorder := harness.chat(imageChatBody); recorder.Code != http.StatusOK {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}

	harness.waitForLoaded(t, "plain.kcpps")
}

func TestWithoutAGuardianTheTextOnlyBackendRefusalReachesTheClient(t *testing.T) {
	harness := newGuardianHarness(t, false)

	recorder := harness.chat(imageChatBody)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "image input is not supported") {
		t.Fatalf("response = %d %s, want the backend's refusal untouched", recorder.Code, recorder.Body.String())
	}
}

func TestTextRequestIsNeverSentToTheGuardian(t *testing.T) {
	harness := newGuardianHarness(t, false)
	harness.assign(harness.clusterGuardian())

	recorder := harness.chat(textChatBody)

	if !strings.Contains(recorder.Body.String(), "answered by plain.kcpps") {
		t.Fatalf("response = %s, want the addressed model to answer a request without images", recorder.Body.String())
	}
}

func TestImageRequestForAModelThatClaimsVisionButRefusesFallsBackToTheGuardian(t *testing.T) {
	harness := newGuardianHarness(t, true)
	harness.assign(guardianmodels.Assignment{
		Scope:    guardianmodels.ModelScope,
		NodeID:   harness.service.nodeID,
		ModelID:  "plain",
		Guardian: guardianmodels.Endpoint{NodeID: harness.service.nodeID, ModelID: "vision"},
	})

	recorder := harness.chat(imageChatBody)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "answered by vision.kcpps") {
		t.Fatalf("response = %d %s, want the guardian to take over after the refusal", recorder.Code, recorder.Body.String())
	}
}

func TestGuardianWithoutVisionIsNotUsed(t *testing.T) {
	harness := newGuardianHarness(t, false)
	harness.assign(guardianmodels.Assignment{
		Scope:    guardianmodels.ClusterScope,
		Guardian: guardianmodels.Endpoint{NodeID: harness.service.nodeID, ModelID: "plain"},
	})

	recorder := harness.chat(imageChatBody)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("response = %d %s, want the refusal when the guardian cannot see either", recorder.Code, recorder.Body.String())
	}
}

func TestServerErrorRefusalOfImagesIsNotRetriedAndFallsBackToTheGuardian(t *testing.T) {
	harness := newGuardianHarness(t, true)
	harness.refusal.Store(http.StatusInternalServerError)
	harness.assign(guardianmodels.Assignment{
		Scope:    guardianmodels.ModelScope,
		NodeID:   harness.service.nodeID,
		ModelID:  "plain",
		Guardian: guardianmodels.Endpoint{NodeID: harness.service.nodeID, ModelID: "vision"},
	})

	recorder := harness.chat(imageChatBody)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "answered by vision.kcpps") {
		t.Fatalf("response = %d %s, want the guardian to take over", recorder.Code, recorder.Body.String())
	}
	if hits := harness.images.Load(); hits != 1 {
		t.Fatalf("the refusing backend saw the image %d times, want once without retries", hits)
	}
}

func TestServerErrorRefusalWithoutAGuardianReachesTheClientAfterOneAttempt(t *testing.T) {
	harness := newGuardianHarness(t, true)
	harness.refusal.Store(http.StatusInternalServerError)

	recorder := harness.chat(imageChatBody)

	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "image input is not supported") {
		t.Fatalf("response = %d %s, want the backend's refusal", recorder.Code, recorder.Body.String())
	}
	if hits := harness.images.Load(); hits != 1 {
		t.Fatalf("the refusing backend saw the image %d times, want once", hits)
	}
}
