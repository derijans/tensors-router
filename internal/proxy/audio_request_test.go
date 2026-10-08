package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAudioSpeechRoutesKnownVoiceConfig(t *testing.T) {
	var readinessProbes atomic.Int32
	var speechRequests atomic.Int32
	wav := append([]byte("RIFF"), make([]byte, 52)...)
	copy(wav[8:], []byte("WAVE"))
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/extra/version":
			ready := readinessProbes.Add(1) > 1
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"result":"KoboldCpp","tts":%t}`, ready)
			return
		case "/v1/models":
			t.Fatal("speech readiness must not use the text model endpoint")
		case "/v1/audio/speech":
			speechRequests.Add(1)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		content, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), `"model":"voice"`) {
			t.Fatalf("request model was not preserved: %s", string(content))
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(wav)))
		_, _ = w.Write(wav)
	}), map[string]string{
		"voice": `{"ttsmodel":"voice.gguf"}`,
	})
	service.backendRetryAttempts = 3
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"voice","input":"hello","voice":"alloy"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "audio/wav" || recorder.Header().Get("Content-Length") != fmt.Sprintf("%d", len(wav)) {
		t.Fatalf("unexpected audio headers content-type=%q content-length=%q", recorder.Header().Get("Content-Type"), recorder.Header().Get("Content-Length"))
	}
	if !bytes.Equal(recorder.Body.Bytes(), wav) {
		t.Fatalf("audio response changed: got %d bytes want %d", recorder.Body.Len(), len(wav))
	}
	if readinessProbes.Load() != 2 || speechRequests.Load() != 1 {
		t.Fatalf("unexpected readiness probes=%d speech requests=%d", readinessProbes.Load(), speechRequests.Load())
	}
	if backend.reloads.Load() != 1 || backend.lastReload != "voice.kcpps" {
		t.Fatalf("expected voice reload, got count=%d config=%q", backend.reloads.Load(), backend.lastReload)
	}
}

func TestAudioSpeechDoesNotForwardUntilTTSCapabilityIsReady(t *testing.T) {
	var speechRequests atomic.Int32
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/extra/version":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":"KoboldCpp","tts":false}`))
		case "/v1/audio/speech":
			speechRequests.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}), map[string]string{
		"voice": `{"ttsmodel":"voice.gguf"}`,
	})
	service.backendRetryAttempts = 2
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"voice","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if speechRequests.Load() != 0 {
		t.Fatalf("speech request reached an unready backend %d times", speechRequests.Load())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
}

func TestAudioSpeechPassesUnknownModelWithoutConfigReload(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		content, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), `"model":"tts-1"`) {
			t.Fatalf("request body changed unexpectedly: %s", string(content))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}), map[string]string{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"tts-1","input":"hello","voice":"alloy"}`))
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("unknown audio model should pass through without reload, got %d", backend.reloads.Load())
	}
}

func TestMusicUIPathPassesThrough(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/musicui" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("music ui"))
	}), map[string]string{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/musicui", nil)
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "music ui" {
		t.Fatalf("unexpected musicui response %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestSplitModeRejectsAudioRoutes(t *testing.T) {
	service, textBackend, imageBackend := newSplitTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("text backend should not receive split audio route")
	}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("image backend should not receive split audio route")
	}), map[string]string{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"voice","input":"hello"}`))
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if textBackend.reloads.Load() != 0 || imageBackend.reloads.Load() != 0 {
		t.Fatalf("split audio should not reload backends text=%d image=%d", textBackend.reloads.Load(), imageBackend.reloads.Load())
	}
}

func TestSplitModeRejectsTextToSpeechRegardlessOfVoiceAssets(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		config string
		body   string
	}{
		{
			name:   "speech with complete talker assets",
			path:   "/v1/audio/speech",
			config: `{"talkermodel":"C:\\models\\talker.gguf","code2wavmodel":"C:\\models\\code2wav.gguf"}`,
			body:   `{"model":"voice","input":"hello","voice":"alloy"}`,
		},
		{
			name:   "speech with incomplete assets",
			path:   "/v1/audio/speech",
			config: `{"code2wavmodel":"C:\\models\\code2wav.gguf"}`,
			body:   `{"model":"voice","input":"hello","voice":"alloy"}`,
		},
		{
			name:   "kobold tts route",
			path:   "/api/extra/tts",
			config: `{"ttsmodel":"C:\\models\\tts.gguf","ttswavtokenizer":"C:\\models\\vocoder.gguf"}`,
			body:   `{"model":"voice","input":"hello"}`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			service, textBackend, imageBackend := newSplitTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("text backend must not receive split-mode speech route %s", r.URL.Path)
			}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("image backend must not receive split-mode speech route %s", r.URL.Path)
			}), map[string]string{
				"voice": testCase.config,
			})

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(testCase.body))
			request.Header.Set("Content-Type", "application/json")
			service.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusNotImplemented {
				t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "text-to-speech is not supported by the split backend") {
				t.Fatalf("rejection did not explain the removal: %s", recorder.Body.String())
			}
			if textBackend.reloads.Load() != 0 || imageBackend.reloads.Load() != 0 {
				t.Fatalf("split-mode speech must not reload backends text=%d image=%d", textBackend.reloads.Load(), imageBackend.reloads.Load())
			}
		})
	}
}

func TestVoiceDiscoveryAndPingCompatibilityRoutes(t *testing.T) {
	var forwarded bool
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/voices" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		forwarded = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))

	voiceRecorder := httptest.NewRecorder()
	service.ServeHTTP(voiceRecorder, httptest.NewRequest(http.MethodGet, "/v1/audio/voices", nil))
	if voiceRecorder.Code != http.StatusOK || !forwarded {
		t.Fatalf("voice discovery failed status=%d body=%s", voiceRecorder.Code, voiceRecorder.Body.String())
	}

	pingRecorder := httptest.NewRecorder()
	service.ServeHTTP(pingRecorder, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if pingRecorder.Code != http.StatusOK || pingRecorder.Body.String() != "{\"status\":\"healthy\"}\n" || backend.reloads.Load() != 0 {
		t.Fatalf("unexpected ping status=%d body=%q reloads=%d", pingRecorder.Code, pingRecorder.Body.String(), backend.reloads.Load())
	}
}

func TestVoiceDiscoveryRejectsSplitBackend(t *testing.T) {
	service, _, _ := newSplitTestServiceWithConfigContents(t, readyTextHandler(t), readyImageHandler(t), map[string]string{
		"voice": `{"talkermodel":"C:\\models\\talker.gguf"}`,
	})
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/audio/voices", nil))
	if recorder.Code != http.StatusNotImplemented || !strings.Contains(recorder.Body.String(), `"type":"unsupported_backend"`) {
		t.Fatalf("unexpected split discovery response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
