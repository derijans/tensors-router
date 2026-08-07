package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerHandshakeAndCapabilityProtocol(t *testing.T) {
	directory := t.TempDir()
	config := DefaultConfig(filepath.Join(directory, "downloader.yaml"))
	config.Logging.Mode = "off"
	var input bytes.Buffer
	if err := writeFrame(&input, protocolRequest{ID: 7, Method: "handshake"}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := ServeWorker(config, &input, &output); err != nil {
		t.Fatal(err)
	}
	var response protocolResponse
	if err := readFrame(&output, &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != 7 || response.Error != "" {
		t.Fatalf("unexpected handshake response %#v", response)
	}
	var handshake Handshake
	if err := json.Unmarshal(response.Result, &handshake); err != nil {
		t.Fatal(err)
	}
	if handshake.Protocol != ProtocolVersion || !containsCapability(handshake.Capabilities, "native_http") || !handshake.Runtime.Available {
		t.Fatalf("unexpected handshake %#v", handshake)
	}
}

func TestWorkerRejectsUnknownMethod(t *testing.T) {
	directory := t.TempDir()
	config := DefaultConfig(filepath.Join(directory, "downloader.yaml"))
	config.Logging.Mode = "off"
	manager, err := NewManager(config, "")
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	_, err = dispatchWorkerRequest(context.Background(), manager, protocolRequest{Method: "execute_shell"})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unexpected unknown-method result %v", err)
	}
}
