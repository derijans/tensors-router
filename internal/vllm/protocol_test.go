package vllm

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type protocolTestService struct {
	closed bool
}

func (service *protocolTestService) State(context.Context) State {
	return State{LifecycleState: LifecycleNeedsInit}
}

func (*protocolTestService) StartInitialization(context.Context, InitRequest) (InitializationJob, error) {
	return InitializationJob{JobID: "job", BackendID: BackendID, State: JobRunning}, nil
}

func (*protocolTestService) CancelInitialization(context.Context) (InitializationJob, error) {
	return InitializationJob{JobID: "job", BackendID: BackendID, State: JobCancelled}, nil
}

func (*protocolTestService) Load(_ context.Context, request RuntimeLoadRequest) (RuntimeStatus, error) {
	return RuntimeStatus{Kind: request.Kind, Running: true}, nil
}

func (*protocolTestService) Restart(_ context.Context, kind RuntimeKind) (RuntimeStatus, error) {
	return RuntimeStatus{Kind: kind, Running: true}, nil
}

func (*protocolTestService) Unload(context.Context, RuntimeKind) error { return nil }

func (*protocolTestService) Runtime(_ context.Context, kind RuntimeKind) (RuntimeStatus, error) {
	return RuntimeStatus{Kind: kind}, nil
}

func (service *protocolTestService) Close() error {
	service.closed = true
	return nil
}

func TestFramedCompanionHandshakeAndRestrictedDispatch(t *testing.T) {
	service := &protocolTestService{}
	var input bytes.Buffer
	if err := writeFrame(&input, protocolRequest{ID: 7, Method: "handshake"}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := ServeWorker(service, &input, &output); err != nil {
		t.Fatal(err)
	}
	if !service.closed {
		t.Fatal("worker did not close resident service")
	}
	var response protocolResponse
	if err := readFrame(&output, &response); err != nil {
		t.Fatal(err)
	}
	var handshake Handshake
	if err := json.Unmarshal(response.Result, &handshake); err != nil {
		t.Fatal(err)
	}
	if response.ID != 7 || handshake.Protocol != ProtocolVersion || !containsString(handshake.Capabilities, "unix_socket") {
		t.Fatalf("unexpected handshake %#v response=%#v", handshake, response)
	}
	if _, err := dispatchWorkerRequest(context.Background(), service, protocolRequest{Method: "execute_shell"}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unexpected unrestricted dispatch error %v", err)
	}
}

func TestProtocolRejectsOversizedFrame(t *testing.T) {
	var frame bytes.Buffer
	frame.Write([]byte{1, 0, 0, 1})
	var target protocolRequest
	if err := readFrame(&frame, &target); err == nil {
		t.Fatal("expected oversized frame rejection")
	}
}
