package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

const (
	maxVLLMResponses       = 10000
	maxVLLMResponseIDBytes = 1024
	vllmResponseLifetime   = 24 * time.Hour
	vllmResponseCleanup    = time.Minute
)

type vllmResponseTarget struct {
	publicID       string
	localID        string
	configFilename string
	remote         bool
	nodeURL        string
}

type vllmResponseEntry struct {
	target    vllmResponseTarget
	expiresAt time.Time
	release   func()
}

type vllmResponseStore struct {
	mu              sync.Mutex
	responses       map[string]vllmResponseEntry
	now             func() time.Time
	cleanupInterval time.Duration
	cleanupStop     chan struct{}
	cleanupDone     chan struct{}
	cleanupStarted  bool
	closeOnce       sync.Once
	closed          bool
}

func newVLLMResponseStore(cleanupInterval ...time.Duration) *vllmResponseStore {
	interval := vllmResponseCleanup
	if len(cleanupInterval) > 0 && cleanupInterval[0] > 0 {
		interval = cleanupInterval[0]
	}
	return &vllmResponseStore{responses: make(map[string]vllmResponseEntry), now: time.Now, cleanupInterval: interval, cleanupStop: make(chan struct{}), cleanupDone: make(chan struct{})}
}

func (store *vllmResponseStore) remember(responseID string, target vllmResponseTarget, release func()) {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" || len(responseID) > maxVLLMResponseIDBytes {
		if release != nil {
			release()
		}
		return
	}
	store.mu.Lock()
	if store.closed {
		store.mu.Unlock()
		if release != nil {
			release()
		}
		return
	}
	releases := store.removeExpiredLocked()
	if previous, exists := store.responses[responseID]; exists && previous.release != nil {
		releases = append(releases, previous.release)
	}
	if len(store.responses) >= maxVLLMResponses {
		if oldestRelease := store.removeOldestLocked(); oldestRelease != nil {
			releases = append(releases, oldestRelease)
		}
	}
	store.responses[responseID] = vllmResponseEntry{target: target, expiresAt: store.now().Add(vllmResponseLifetime), release: release}
	store.scheduleCleanupLocked()
	store.mu.Unlock()
	releaseVLLMResponseLeases(releases)
}

func (store *vllmResponseStore) target(responseID string) (vllmResponseTarget, bool) {
	store.mu.Lock()
	releases := store.removeExpiredLocked()
	entry, found := store.responses[strings.TrimSpace(responseID)]
	store.mu.Unlock()
	releaseVLLMResponseLeases(releases)
	return entry.target, found
}

func (store *vllmResponseStore) forget(responseID string) {
	store.mu.Lock()
	var release func()
	if entry, found := store.responses[strings.TrimSpace(responseID)]; found {
		delete(store.responses, strings.TrimSpace(responseID))
		release = entry.release
	}
	store.mu.Unlock()
	releaseVLLMResponseLeases([]func(){release})
}

func (store *vllmResponseStore) close() {
	store.closeOnce.Do(func() {
		store.mu.Lock()
		store.closed = true
		cleanupStarted := store.cleanupStarted
		if cleanupStarted {
			close(store.cleanupStop)
		}
		releases := make([]func(), 0, len(store.responses))
		for responseID, entry := range store.responses {
			delete(store.responses, responseID)
			if entry.release != nil {
				releases = append(releases, entry.release)
			}
		}
		store.mu.Unlock()
		if cleanupStarted {
			<-store.cleanupDone
		}
		releaseVLLMResponseLeases(releases)
	})
}

func (store *vllmResponseStore) scheduleCleanupLocked() {
	if !store.cleanupStarted && !store.closed {
		store.cleanupStarted = true
		go store.cleanupExpired()
	}
}

func (store *vllmResponseStore) cleanupExpired() {
	ticker := time.NewTicker(store.cleanupInterval)
	defer ticker.Stop()
	defer close(store.cleanupDone)
	for {
		select {
		case <-ticker.C:
			store.mu.Lock()
			releases := store.removeExpiredLocked()
			store.mu.Unlock()
			releaseVLLMResponseLeases(releases)
		case <-store.cleanupStop:
			return
		}
	}
}

func (store *vllmResponseStore) removeExpiredLocked() []func() {
	now := store.now()
	releases := make([]func(), 0)
	for responseID, entry := range store.responses {
		if !now.Before(entry.expiresAt) {
			delete(store.responses, responseID)
			if entry.release != nil {
				releases = append(releases, entry.release)
			}
		}
	}
	return releases
}

func (store *vllmResponseStore) removeOldestLocked() func() {
	oldestID := ""
	var oldestExpiry time.Time
	for responseID, entry := range store.responses {
		if oldestID == "" || entry.expiresAt.Before(oldestExpiry) {
			oldestID = responseID
			oldestExpiry = entry.expiresAt
		}
	}
	if oldestID != "" {
		entry := store.responses[oldestID]
		delete(store.responses, oldestID)
		return entry.release
	}
	return nil
}

func releaseVLLMResponseLeases(releases []func()) {
	for _, release := range releases {
		if release != nil {
			release()
		}
	}
}

func (service *Service) handleVLLMResponseOperation(w http.ResponseWriter, r *http.Request) {
	responseID, action, ok := vllmResponseOperation(r.URL.Path)
	if !ok || !vllmInferenceAllowed(r.Method, r.URL.Path) {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	target, found := service.vllmResponses.target(responseID)
	if !found {
		openai.WriteError(w, http.StatusNotFound, "response_not_found", "response ownership is unknown")
		return
	}
	body, ok := service.readVLLMRequestBody(w, r)
	if !ok {
		return
	}
	var response *http.Response
	var err error
	if target.remote {
		response, err = service.forwardRemote(r.Context(), r, body, cluster.Route{NodeURL: target.nodeURL, Remote: true})
	} else {
		response, _, err = service.forwardWithFallbackObserved(r.Context(), r, body, target.localID, target.configFilename, true, readinessText, BackendModeVLLM)
	}
	if err != nil {
		writeBackendFailure(w, err)
		return
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 && (action == "cancel" || r.Method == http.MethodDelete) {
		service.vllmResponses.forget(responseID)
	}
	if err := service.writeModelProxyResponse(w, response, target.publicID, true); err != nil {
		service.logger.Printf("vLLM response operation failed response=%q error=%v", responseID, err)
	}
}

func (service *Service) responseWithVLLMTracking(response *http.Response, target vllmResponseTarget) *http.Response {
	if response == nil || response.Body == nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return response
	}
	remember := func(responseID string) {
		var release func()
		if !target.remote {
			if runtime, err := service.runtimeForBackendMode(BackendModeVLLM, readinessText); err == nil && runtime != nil {
				release, _ = acquireLoadedRuntimeLease(service, runtime)
			}
		}
		service.vllmResponses.remember(responseID, target, release)
	}
	if isEventStream(response.Header) {
		response.Body = newVLLMResponseEventStream(response.Body, remember)
		return response
	}
	if !isJSONResponse(response.Header) || response.ContentLength > backendResponseMetadataLimit {
		return response
	}
	originalBody := response.Body
	body, err := io.ReadAll(io.LimitReader(originalBody, backendResponseMetadataLimit+1))
	if err != nil {
		_ = originalBody.Close()
		response.Body = io.NopCloser(bytes.NewReader(nil))
		response.ContentLength = 0
		return response
	}
	if len(body) > backendResponseMetadataLimit {
		response.Body = replayReadCloser{Reader: io.MultiReader(bytes.NewReader(body), originalBody), closer: originalBody}
		return response
	}
	_ = originalBody.Close()
	if responseID := vllmResponseID(body); responseID != "" {
		remember(responseID)
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	return response
}

type vllmResponseEventStream struct {
	reader   *bufio.Reader
	closer   io.Closer
	remember func(string)
	pending  []byte
	tracked  bool
}

func newVLLMResponseEventStream(body io.ReadCloser, remember func(string)) io.ReadCloser {
	return &vllmResponseEventStream{reader: bufio.NewReader(body), closer: body, remember: remember}
}

func (stream *vllmResponseEventStream) Read(destination []byte) (int, error) {
	count, err := stream.reader.Read(destination)
	if count > 0 && !stream.tracked && len(stream.pending) <= backendResponseMetadataLimit-count {
		stream.pending = append(stream.pending, destination[:count]...)
		if responseID := vllmResponseIDFromEvent(stream.pending); responseID != "" {
			stream.remember(responseID)
			stream.tracked = true
			stream.pending = nil
		}
	}
	return count, err
}

func (stream *vllmResponseEventStream) Close() error {
	return stream.closer.Close()
}

func vllmResponseIDFromEvent(chunk []byte) string {
	for _, line := range bytes.Split(chunk, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		if responseID := vllmResponseID(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))); responseID != "" {
			return responseID
		}
	}
	return ""
}

func vllmResponseID(body []byte) string {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return ""
	}
	return findVLLMResponseID(value)
}

func findVLLMResponseID(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		if id, ok := typed["id"].(string); ok && strings.HasPrefix(strings.TrimSpace(id), "resp") {
			return strings.TrimSpace(id)
		}
		for _, key := range []string{"response", "data"} {
			if responseID := findVLLMResponseID(typed[key]); responseID != "" {
				return responseID
			}
		}
	}
	return ""
}
