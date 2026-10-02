package downloader

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"tensors-router/internal/processcontrol"
)

const (
	startupHandshakeTimeout  = 20 * time.Second
	companionShutdownTimeout = 2 * time.Second
	companionKillWaitTimeout = 2 * time.Second
	defaultCallTimeout       = 30 * time.Second
	rescanCallTimeout        = 10 * time.Minute
)

var requiredCompanionCapabilities = []string{"native_http", "sha256", "artifact_events"}

type Client struct {
	command    *exec.Cmd
	input      io.WriteCloser
	writeMu    sync.Mutex
	stateMu    sync.Mutex
	pending    map[uint64]chan protocolResponse
	nextID     atomic.Uint64
	done       chan struct{}
	waitError  error
	closeError error
	capability Capability
	artifacts  *artifactQueue
	closeOnce  sync.Once
	stderr     *boundedBuffer
}

func StartClient(ctx context.Context, binaryPath string, configPath string) (*Client, error) {
	absoluteConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, err
	}
	command := exec.Command(binaryPath, "worker", "--config", absoluteConfigPath)
	processcontrol.Prepare(command, processcontrol.Options{HideWindow: true})
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &boundedBuffer{limit: 16 << 10}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	client := newClient(command, input, stderr)
	go client.readResponses(output)
	handshakeContext, cancel := context.WithTimeout(ctx, startupHandshakeTimeout)
	defer cancel()
	var handshake Handshake
	if err := client.call(handshakeContext, "handshake", nil, &handshake); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("downloader companion handshake failed: %w", err)
	}
	if handshake.Protocol != ProtocolVersion {
		_ = client.Close()
		return nil, fmt.Errorf("downloader companion protocol %d is incompatible with required protocol %d; install matching router and downloader builds", handshake.Protocol, ProtocolVersion)
	}
	for _, required := range requiredCompanionCapabilities {
		if !slices.Contains(handshake.Capabilities, required) {
			_ = client.Close()
			return nil, fmt.Errorf("downloader companion does not provide required capability %q", required)
		}
	}
	client.capability = handshake.Runtime
	return client, nil
}

func newClient(command *exec.Cmd, input io.WriteCloser, stderr *boundedBuffer) *Client {
	return &Client{command: command, input: input, pending: map[uint64]chan protocolResponse{}, done: make(chan struct{}), stderr: stderr, artifacts: newArtifactQueue()}
}

func (client *Client) Capability() Capability {
	var capability Capability
	if err := client.callWithTimeout("capability", nil, &capability, 2*time.Second); err != nil {
		client.stateMu.Lock()
		capability = client.capability
		client.stateMu.Unlock()
		capability.Available = false
		capability.Error = err.Error()
		capability.Reason = capability.Error
		return capability
	}
	client.stateMu.Lock()
	client.capability = capability
	client.stateMu.Unlock()
	return capability
}

func (client *Client) Search(ctx context.Context, request SearchRequest, token string) ([]SearchResult, error) {
	var result []SearchResult
	err := client.call(ctx, "search", searchCall{Request: request, Token: token}, &result)
	return result, err
}

func (client *Client) SearchPage(ctx context.Context, request SearchRequest, token string) (SearchPage, error) {
	var result SearchPage
	err := client.call(ctx, "search_page", searchCall{Request: request, Token: token}, &result)
	return result, err
}

func (client *Client) Repository(ctx context.Context, request RepositoryRequest) (RepositoryDetails, error) {
	var result RepositoryDetails
	err := client.call(ctx, "repository", request, &result)
	return result, err
}

func (client *Client) Plan(ctx context.Context, request PlanRequest) (DownloadPlan, error) {
	var result DownloadPlan
	err := client.call(ctx, "plan", request, &result)
	return result, err
}

func (client *Client) CreateJob(ctx context.Context, request CreateJobRequest) (DownloadJob, error) {
	var result DownloadJob
	err := client.call(ctx, "create_job", request, &result)
	return result, err
}

func (client *Client) Job(id string) (DownloadJob, bool, error) {
	var result jobResult
	err := client.callWithTimeout("job", idCall{ID: id}, &result, defaultCallTimeout)
	return result.Job, result.Found, err
}

func (client *Client) Jobs() ([]DownloadJob, error) {
	var result []DownloadJob
	err := client.callWithTimeout("jobs", nil, &result, defaultCallTimeout)
	return result, err
}

func (client *Client) Artifacts() ([]ArtifactRecord, error) {
	var result []ArtifactRecord
	err := client.callWithTimeout("artifacts", nil, &result, defaultCallTimeout)
	return result, err
}

func (client *Client) Pause(id string) (DownloadJob, error) {
	return client.jobAction("pause", id)
}

func (client *Client) Resume(id string) (DownloadJob, error) {
	return client.jobAction("resume", id)
}

func (client *Client) Cancel(id string) (DownloadJob, error) {
	return client.jobAction("cancel", id)
}

func (client *Client) jobAction(method string, id string) (DownloadJob, error) {
	var result DownloadJob
	err := client.callWithTimeout(method, idCall{ID: id}, &result, defaultCallTimeout)
	return result, err
}

func (client *Client) Rescan() ([]ArtifactRecord, error) {
	var result []ArtifactRecord
	err := client.callWithTimeout("rescan", nil, &result, rescanCallTimeout)
	return result, err
}

func (client *Client) SetArtifactHandler(handler ArtifactHandler) {
	client.artifacts.setHandler(handler)
}

func (client *Client) Done() <-chan struct{} {
	return client.done
}

func (client *Client) Close() error {
	client.closeOnce.Do(func() {
		inputError := client.input.Close()
		var shutdownError error
		if !waitForCompanion(client.done, companionShutdownTimeout) {
			shutdownError = fmt.Errorf("downloader companion did not exit within %s", companionShutdownTimeout)
			if killError := client.command.Process.Kill(); killError != nil && !errors.Is(killError, os.ErrProcessDone) {
				shutdownError = errors.Join(shutdownError, fmt.Errorf("kill downloader companion: %w", killError))
			}
			if !waitForCompanion(client.done, companionKillWaitTimeout) {
				shutdownError = errors.Join(shutdownError, fmt.Errorf("downloader companion did not terminate within %s after kill", companionKillWaitTimeout))
			}
		}
		client.artifacts.close()
		client.stateMu.Lock()
		client.closeError = errors.Join(inputError, shutdownError)
		client.stateMu.Unlock()
	})
	client.stateMu.Lock()
	defer client.stateMu.Unlock()
	return errors.Join(client.closeError, client.waitError)
}

func waitForCompanion(done <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func (client *Client) callWithTimeout(method string, payload any, target any, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return client.call(ctx, method, payload, target)
}

func (client *Client) call(ctx context.Context, method string, payload any, target any) error {
	var content json.RawMessage
	var err error
	if payload != nil {
		content, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	id := client.nextID.Add(1)
	responseChannel := make(chan protocolResponse, 1)
	client.stateMu.Lock()
	select {
	case <-client.done:
		client.stateMu.Unlock()
		return client.connectionError()
	default:
		client.pending[id] = responseChannel
	}
	client.stateMu.Unlock()
	client.writeMu.Lock()
	err = writeFrame(client.input, protocolRequest{ID: id, Method: method, Payload: content})
	client.writeMu.Unlock()
	if err != nil {
		client.removePending(id)
		return fmt.Errorf("write downloader companion request: %w", err)
	}
	select {
	case response := <-responseChannel:
		if response.Error != "" {
			if response.Code != "" {
				return &Error{Code: response.Code, Message: response.Error}
			}
			return errors.New(response.Error)
		}
		if target == nil {
			return nil
		}
		if err := json.Unmarshal(response.Result, target); err != nil {
			return fmt.Errorf("decode downloader companion response: %w", err)
		}
		return nil
	case <-ctx.Done():
		client.removePending(id)
		return ctx.Err()
	case <-client.done:
		client.removePending(id)
		return client.connectionError()
	}
}

func (client *Client) readResponses(output io.Reader) {
	reader := bufio.NewReader(output)
	for {
		var response protocolResponse
		if err := readFrame(reader, &response); err != nil {
			client.finish(err)
			return
		}
		if response.Event == artifactEvent {
			var record ArtifactRecord
			if json.Unmarshal(response.Result, &record) == nil {
				client.artifacts.push(record)
			}
			continue
		}
		client.stateMu.Lock()
		channel := client.pending[response.ID]
		delete(client.pending, response.ID)
		client.stateMu.Unlock()
		if channel != nil {
			channel <- response
		}
	}
}

func (client *Client) finish(readError error) {
	waitError := client.command.Wait()
	if errors.Is(readError, io.EOF) {
		readError = nil
	}
	client.stateMu.Lock()
	client.waitError = errors.Join(readError, waitError)
	close(client.done)
	client.stateMu.Unlock()
}

func (client *Client) connectionError() error {
	client.stateMu.Lock()
	waitError := client.waitError
	client.stateMu.Unlock()
	message := "downloader companion terminated"
	if detail := client.stderr.String(); detail != "" {
		message += ": " + redactSensitive(detail)
	}
	if waitError != nil {
		return fmt.Errorf("%s: %w", message, waitError)
	}
	return errors.New(message)
}

func (client *Client) removePending(id uint64) {
	client.stateMu.Lock()
	delete(client.pending, id)
	client.stateMu.Unlock()
}

type boundedBuffer struct {
	mu    sync.Mutex
	data  []byte
	limit int
}

func (buffer *boundedBuffer) Write(content []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	available := buffer.limit - len(buffer.data)
	if available > 0 {
		if available > len(content) {
			available = len(content)
		}
		buffer.data = append(buffer.data, content[:available]...)
	}
	return len(content), nil
}

func (buffer *boundedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return string(bytes.TrimSpace(buffer.data))
}
