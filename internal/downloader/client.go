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
	"sync"
	"sync/atomic"
	"time"
)

const (
	startupHandshakeTimeout  = 5 * time.Second
	companionShutdownTimeout = 2 * time.Second
	companionKillWaitTimeout = 2 * time.Second
	subscriptionPollInterval = 250 * time.Millisecond
)

type Client struct {
	command         *exec.Cmd
	input           io.WriteCloser
	writeMu         sync.Mutex
	stateMu         sync.Mutex
	pending         map[uint64]chan protocolResponse
	nextID          atomic.Uint64
	done            chan struct{}
	waitError       error
	closeError      error
	capability      Capability
	artifactHandler ArtifactHandler
	closeOnce       sync.Once
	stderr          *boundedBuffer
}

var _ Service = (*Client)(nil)

func StartClient(ctx context.Context, binaryPath string, configPath string) (*Client, error) {
	command := exec.Command(binaryPath, "worker", "--config", configPath)
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
	client := &Client{command: command, input: input, pending: map[uint64]chan protocolResponse{}, done: make(chan struct{}), stderr: stderr}
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
		return nil, fmt.Errorf("downloader companion protocol %d is incompatible with required protocol %d", handshake.Protocol, ProtocolVersion)
	}
	if !containsCapability(handshake.Capabilities, "native_http") || !containsCapability(handshake.Capabilities, "sha256") {
		_ = client.Close()
		return nil, fmt.Errorf("downloader companion does not provide required native transfer capabilities")
	}
	client.capability = handshake.Runtime
	return client, nil
}

func (client *Client) Capability() Capability {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var capability Capability
	if err := client.call(ctx, "capability", nil, &capability); err != nil {
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
	err := client.call(context.Background(), "job", idCall{ID: id}, &result)
	return result.Job, result.Found, err
}

func (client *Client) Jobs() ([]DownloadJob, error) {
	var result []DownloadJob
	err := client.call(context.Background(), "jobs", nil, &result)
	return result, err
}

func (client *Client) Artifacts() ([]ArtifactRecord, error) {
	var result []ArtifactRecord
	err := client.call(context.Background(), "artifacts", nil, &result)
	return result, err
}

func (client *Client) Pause(id string) (DownloadJob, error) {
	var result DownloadJob
	err := client.call(context.Background(), "pause", idCall{ID: id}, &result)
	return result, err
}

func (client *Client) Resume(id string) (DownloadJob, error) {
	var result DownloadJob
	err := client.call(context.Background(), "resume", idCall{ID: id}, &result)
	return result, err
}

func (client *Client) Cancel(id string) (DownloadJob, error) {
	var result DownloadJob
	err := client.call(context.Background(), "cancel", idCall{ID: id}, &result)
	return result, err
}

func (client *Client) Subscribe(id string) (<-chan DownloadJob, func()) {
	events := make(chan DownloadJob, 8)
	stop := make(chan struct{})
	var stopOnce sync.Once
	go client.pollJob(id, events, stop)
	return events, func() { stopOnce.Do(func() { close(stop) }) }
}

func (client *Client) Rescan() ([]ArtifactRecord, error) {
	var result []ArtifactRecord
	if err := client.call(context.Background(), "rescan", nil, &result); err != nil {
		return nil, err
	}
	client.notifyArtifacts(result)
	return result, nil
}

func (client *Client) SetArtifactHandler(handler ArtifactHandler) {
	client.stateMu.Lock()
	client.artifactHandler = handler
	client.stateMu.Unlock()
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

func (client *Client) pollJob(id string, events chan<- DownloadJob, stop <-chan struct{}) {
	defer close(events)
	ticker := time.NewTicker(subscriptionPollInterval)
	defer ticker.Stop()
	var lastUpdate time.Time
	for {
		job, found, err := client.Job(id)
		if err != nil || !found {
			return
		}
		if !job.UpdatedAt.Equal(lastUpdate) {
			select {
			case events <- job:
				lastUpdate = job.UpdatedAt
			case <-stop:
				return
			}
		}
		if job.State == JobCompleted || job.State == JobFailed || job.State == JobCancelled {
			if job.State == JobCompleted {
				client.notifyCompletedJob(job)
			}
			return
		}
		select {
		case <-ticker.C:
		case <-stop:
			return
		case <-client.done:
			return
		}
	}
}

func (client *Client) notifyCompletedJob(job DownloadJob) {
	artifacts, err := client.Artifacts()
	if err != nil {
		return
	}
	matches := make([]ArtifactRecord, 0, len(job.Files))
	for _, artifact := range artifacts {
		if artifact.Repository == job.Repository && artifact.Revision == job.Commit {
			matches = append(matches, artifact)
		}
	}
	client.notifyArtifacts(matches)
}

func (client *Client) notifyArtifacts(artifacts []ArtifactRecord) {
	client.stateMu.Lock()
	handler := client.artifactHandler
	client.stateMu.Unlock()
	if handler == nil {
		return
	}
	for _, artifact := range artifacts {
		_ = handler(artifact)
	}
}

func containsCapability(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
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
