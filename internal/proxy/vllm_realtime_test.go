package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

func TestRealtimeHeadersStripCredentials(t *testing.T) {
	source := http.Header{
		"Connection":          []string{"Upgrade"},
		"Upgrade":             []string{"websocket"},
		"Sec-Websocket-Key":   []string{"key"},
		"Authorization":       []string{"Bearer secret"},
		"Proxy-Authorization": []string{"Basic secret"},
		"Cookie":              []string{"session=secret"},
	}
	destination := http.Header{}
	copyRealtimeHeaders(destination, source)
	if destination.Get("Connection") != "Upgrade" || destination.Get("Sec-WebSocket-Key") != "key" {
		t.Fatalf("WebSocket handshake headers were lost: %#v", destination)
	}
	for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie"} {
		if destination.Get(name) != "" {
			t.Fatalf("credential header %s reached backend", name)
		}
	}
}

func TestVLLMRealtimeProxiesConnectionAndHoldsLease(t *testing.T) {
	upgraded := make(chan struct{}, 1)
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/v1/realtime" || r.URL.Query().Get("model") != "speech-adapter" {
			t.Errorf("unexpected backend request %s?%s", r.URL.Path, r.URL.RawQuery)
			return
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Errorf("credentials reached backend: %#v", r.Header)
			return
		}
		connection, buffer, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("backend hijack: %v", err)
			return
		}
		defer connection.Close()
		_, _ = buffer.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: accepted\r\n\r\n")
		_ = buffer.Flush()
		upgraded <- struct{}{}
		payload := make([]byte, 5)
		if _, err := io.ReadFull(connection, payload); err == nil {
			_, _ = connection.Write(payload)
		}
	}))
	defer backendServer.Close()
	backendURL, err := url.Parse(backendServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "speech.kcpps"), []byte(`{"backend_mode":"vllm","vllm":{"task":"speech","served_names":["speech-adapter"]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		BackendMode: BackendModeVLLM,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeVLLM: {TextBackend: backend, EmbeddingsBackend: backend, TranscriptionBackend: backend},
		},
		Catalog:   catalog.New(configDir),
		ConfigDir: configDir,
		Logger:    log.New(io.Discard, "", 0),
	})
	routerServer := httptest.NewServer(service)
	defer routerServer.Close()
	routerURL, err := url.Parse(routerServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("tcp", routerURL.Host, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	request := fmt.Sprintf("GET /v1/realtime?model=speech-adapter HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Key: key\r\nSec-WebSocket-Version: 13\r\nAuthorization: Bearer secret\r\nCookie: session=secret\r\n\r\n", routerURL.Host)
	if _, err := io.WriteString(connection, request); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(connection)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(statusLine, "101") {
		rest, _ := io.ReadAll(reader)
		t.Fatalf("unexpected upgrade response %q %s", statusLine, rest)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	select {
	case <-upgraded:
	case <-time.After(5 * time.Second):
		t.Fatal("backend was not upgraded")
	}
	service.transcriptionRuntime.state.mu.Lock()
	users := service.transcriptionRuntime.state.users
	service.transcriptionRuntime.state.mu.Unlock()
	if users == 0 {
		t.Fatal("Realtime connection did not hold a runtime lease")
	}
	if _, err := connection.Write([]byte("frame")); err != nil {
		t.Fatal(err)
	}
	echo := make([]byte, 5)
	if _, err := io.ReadFull(reader, echo); err != nil {
		t.Fatal(err)
	}
	if string(echo) != "frame" {
		t.Fatalf("unexpected echoed payload %q", echo)
	}
}

func TestRemoteVLLMRealtimePreservesServedName(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/router/v1/node/inference/v1/realtime" || r.URL.Query().Get("model") != "speech-adapter" {
			t.Errorf("unexpected remote target %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("cluster authorization missing: %#v", r.Header)
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer backend.Close()
	service := NewService(ServiceConfig{ClusterToken: "secret", ClusterClient: cluster.NewClient("secret", backend.URL), Logger: log.New(io.Discard, "", 0)})
	defer service.Close(t.Context())
	request := httptest.NewRequest(http.MethodGet, "/v1/realtime?model=speech-adapter", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	response, err := service.openRemoteRealtime(request, cluster.Route{NodeURL: backend.URL, Remote: true}, "speech-adapter")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected remote response status %d", response.StatusCode)
	}
}

func TestRealtimeStreamsAreByteBounded(t *testing.T) {
	clientRouter, clientPeer := net.Pipe()
	backendRouter, backendPeer := net.Pipe()
	defer clientPeer.Close()
	defer backendPeer.Close()
	done := make(chan struct{})
	go func() {
		proxyBidirectionalStream(context.Background(), clientRouter, backendRouter, 4, 4)
		close(done)
	}()
	go func() {
		_, _ = clientPeer.Write([]byte("12345678"))
	}()
	payload, err := io.ReadAll(backendPeer)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "1234" {
		t.Fatalf("client stream exceeded limit: %q", payload)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("bounded Realtime proxy did not close connections")
	}
}
