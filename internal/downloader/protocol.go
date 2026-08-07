package downloader

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

const (
	ProtocolVersion = 1
	maxFrameBytes   = 32 << 20
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
	Protocol     int        `json:"protocol"`
	Capabilities []string   `json:"capabilities"`
	Runtime      Capability `json:"runtime"`
}

func ServeWorker(config Config, input io.Reader, output io.Writer) error {
	manager, err := NewManager(config, "")
	if err != nil {
		return err
	}
	defer manager.Close()
	reader := bufio.NewReader(input)
	writer := bufio.NewWriter(output)
	var writeMu sync.Mutex
	var workers sync.WaitGroup
	workerContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		var request protocolRequest
		if err := readFrame(reader, &request); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				workers.Wait()
				return nil
			}
			return err
		}
		workers.Add(1)
		go func(request protocolRequest) {
			defer workers.Done()
			result, callErr := dispatchWorkerRequest(workerContext, manager, request)
			response := protocolResponse{ID: request.ID, Error: redactSensitive(errorText(callErr))}
			if callErr == nil {
				response.Result, callErr = json.Marshal(result)
				if callErr != nil {
					response.Error = callErr.Error()
				}
			}
			writeMu.Lock()
			defer writeMu.Unlock()
			if err := writeFrame(writer, response); err == nil {
				_ = writer.Flush()
			} else {
				cancel()
			}
		}(request)
	}
}

func dispatchWorkerRequest(ctx context.Context, manager *Manager, request protocolRequest) (any, error) {
	switch request.Method {
	case "handshake":
		return Handshake{Protocol: ProtocolVersion, Capabilities: []string{"native_http", "range_resume", "sha256", "jobs", "artifacts"}, Runtime: manager.Capability()}, nil
	case "capability":
		return manager.Capability(), nil
	case "search":
		var value searchCall
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return manager.Search(ctx, value.Request, value.Token)
	case "search_page":
		var value searchCall
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return manager.SearchPage(ctx, value.Request, value.Token)
	case "repository":
		var value RepositoryRequest
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return manager.Repository(ctx, value)
	case "plan":
		var value PlanRequest
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return manager.Plan(ctx, value)
	case "create_job":
		var value CreateJobRequest
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return manager.CreateJob(ctx, value)
	case "job":
		var value idCall
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		job, found, err := manager.Job(value.ID)
		return jobResult{Job: job, Found: found}, err
	case "jobs":
		return manager.Jobs()
	case "artifacts":
		return manager.Artifacts()
	case "pause":
		var value idCall
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return manager.Pause(value.ID)
	case "resume":
		var value idCall
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return manager.Resume(value.ID)
	case "cancel":
		var value idCall
		if err := decodePayload(request.Payload, &value); err != nil {
			return nil, err
		}
		return manager.Cancel(value.ID)
	case "rescan":
		return manager.Rescan()
	default:
		return nil, fmt.Errorf("unsupported downloader worker method %q", request.Method)
	}
}

type searchCall struct {
	Request SearchRequest `json:"request"`
	Token   string        `json:"token,omitempty"`
}

type idCall struct {
	ID string `json:"id"`
}

type jobResult struct {
	Job   DownloadJob `json:"job"`
	Found bool        `json:"found"`
}

func decodePayload(payload json.RawMessage, target any) error {
	if len(payload) == 0 {
		return fmt.Errorf("downloader worker request payload is required")
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("decode downloader worker request: %w", err)
	}
	return nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func readFrame(reader io.Reader, target any) error {
	var size uint32
	if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
		return err
	}
	if size == 0 || size > maxFrameBytes {
		return fmt.Errorf("invalid downloader protocol frame size %d", size)
	}
	content := make([]byte, size)
	if _, err := io.ReadFull(reader, content); err != nil {
		return err
	}
	if err := json.Unmarshal(content, target); err != nil {
		return fmt.Errorf("decode downloader protocol frame: %w", err)
	}
	return nil
}

func writeFrame(writer io.Writer, value any) error {
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(content) == 0 || len(content) > maxFrameBytes {
		return fmt.Errorf("invalid downloader protocol frame size %d", len(content))
	}
	if err := binary.Write(writer, binary.BigEndian, uint32(len(content))); err != nil {
		return err
	}
	_, err = writer.Write(content)
	return err
}
