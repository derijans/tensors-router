package vllm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

const (
	ProtocolVersion   = 1
	maximumFrameBytes = 8 << 20
)

type protocolRequest struct {
	ID      uint64          `json:"id"`
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type protocolResponse struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type Handshake struct {
	Protocol     int      `json:"protocol"`
	Capabilities []string `json:"capabilities"`
	State        State    `json:"state"`
}

func ServeWorker(service Service, input io.Reader, output io.Writer) error {
	reader := bufio.NewReader(input)
	writer := bufio.NewWriter(output)
	workerContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer service.Close()
	var writeMutex sync.Mutex
	var workers sync.WaitGroup
	for {
		var request protocolRequest
		if err := readFrame(reader, &request); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				cancel()
				workers.Wait()
				return nil
			}
			return err
		}
		workers.Add(1)
		go func(request protocolRequest) {
			defer workers.Done()
			result, callError := dispatchWorkerRequest(workerContext, service, request)
			response := protocolResponse{ID: request.ID, Error: errorText(sanitizeError(callError))}
			if callError == nil {
				response.Result, callError = json.Marshal(result)
				if callError != nil {
					response.Error = callError.Error()
				}
			}
			writeMutex.Lock()
			defer writeMutex.Unlock()
			if err := writeFrame(writer, response); err != nil {
				cancel()
				return
			}
			_ = writer.Flush()
		}(request)
	}
}

func dispatchWorkerRequest(ctx context.Context, service Service, request protocolRequest) (any, error) {
	switch request.Method {
	case "handshake":
		return Handshake{Protocol: ProtocolVersion, Capabilities: workerCapabilities(), State: service.State(ctx)}, nil
	case "state":
		return service.State(ctx), nil
	case "initialize":
		var value InitRequest
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return service.StartInitialization(ctx, value)
	case "cancel_initialization":
		return service.CancelInitialization(ctx)
	case "load":
		var value RuntimeLoadRequest
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return service.Load(ctx, value)
	case "restart":
		var value runtimeCall
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return service.Restart(ctx, value.Kind)
	case "unload":
		var value runtimeCall
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return nil, service.Unload(ctx, value.Kind)
	case "launch_options":
		return service.LaunchOptions(ctx)
	case "set_launch_options":
		var value LaunchOptions
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return service.SetLaunchOptions(ctx, value)
	case "runtime":
		var value runtimeCall
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return service.Runtime(ctx, value.Kind)
	default:
		return nil, fmt.Errorf("unsupported vLLM companion method %q", request.Method)
	}
}

func workerCapabilities() []string {
	capabilities := []string{"persistent_jobs", "atomic_environments", "generation", "pooling", "speech", "unix_socket"}
	if embeddedUVAvailable() {
		capabilities = append(capabilities, "embedded_uv")
	}
	return capabilities
}

type runtimeCall struct {
	Kind RuntimeKind `json:"kind"`
}

func decodePayload(payload json.RawMessage, target any) error {
	if len(payload) == 0 {
		return fmt.Errorf("vLLM companion request payload is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode vLLM companion request: %w", err)
	}
	return requireJSONEOF(decoder)
}

func readFrame(reader io.Reader, target any) error {
	var size uint32
	if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
		return err
	}
	if size == 0 || size > maximumFrameBytes {
		return fmt.Errorf("invalid vLLM protocol frame size %d", size)
	}
	content := make([]byte, size)
	if _, err := io.ReadFull(reader, content); err != nil {
		return err
	}
	if err := json.Unmarshal(content, target); err != nil {
		return fmt.Errorf("decode vLLM protocol frame: %w", err)
	}
	return nil
}

func writeFrame(writer io.Writer, value any) error {
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(content) == 0 || len(content) > maximumFrameBytes {
		return fmt.Errorf("invalid vLLM protocol frame size %d", len(content))
	}
	if err := binary.Write(writer, binary.BigEndian, uint32(len(content))); err != nil {
		return err
	}
	_, err = writer.Write(content)
	return err
}
