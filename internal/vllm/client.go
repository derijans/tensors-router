package vllm

import (
	"bufio"
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

	"tensors-router/internal/processcontrol"
)

const (
	companionHandshakeTimeout = 10 * time.Second
	companionShutdownTimeout  = 10 * time.Second
)

type ClientConfig struct {
	DataDir                 string
	DefaultProfile          string
	ManifestPath            string
	ManifestSize            int64
	ManifestSHA256          string
	TUFRepositoryURL        string
	TUFRootPath             string
	AllowTrustRemoteCode    bool
	AllowExternalTools      bool
	AllowDynamicLoRA        bool
	AllowUnverifiedInstall  bool
	UnverifiedVLLMVersion   string
	UnverifiedPythonVersion string
	UnverifiedIndexURL      string
	UnverifiedExtraIndexURL string
}

type Client struct {
	command   *exec.Cmd
	input     io.WriteCloser
	writeMu   sync.Mutex
	stateMu   sync.Mutex
	pending   map[uint64]chan protocolResponse
	nextID    atomic.Uint64
	done      chan struct{}
	waitError error
	closeOnce sync.Once
	logs      *boundedLog
	state     State
}

var _ Service = (*Client)(nil)

func StartClient(ctx context.Context, binaryPath string, configuration ClientConfig) (*Client, error) {
	arguments := []string{"worker", "--data-dir", configuration.DataDir, "--profile", configuration.DefaultProfile}
	if configuration.ManifestPath != "" {
		arguments = append(arguments, "--manifest", configuration.ManifestPath)
	}
	if configuration.TUFRepositoryURL != "" {
		arguments = append(arguments, "--tuf-repository-url", configuration.TUFRepositoryURL)
		if configuration.TUFRootPath != "" {
			arguments = append(arguments, "--tuf-root", configuration.TUFRootPath)
		}
	} else if configuration.ManifestSHA256 != "" || configuration.ManifestSize != 0 {
		arguments = append(arguments, "--manifest-size", fmt.Sprint(configuration.ManifestSize), "--manifest-sha256", configuration.ManifestSHA256)
	}
	if configuration.AllowUnverifiedInstall {
		arguments = append(arguments, "--allow-unverified-install", "true")
		if configuration.UnverifiedVLLMVersion != "" {
			arguments = append(arguments, "--unverified-vllm-version", configuration.UnverifiedVLLMVersion)
		}
		if configuration.UnverifiedPythonVersion != "" {
			arguments = append(arguments, "--unverified-python-version", configuration.UnverifiedPythonVersion)
		}
		if configuration.UnverifiedIndexURL != "" {
			arguments = append(arguments, "--unverified-index-url", configuration.UnverifiedIndexURL)
		}
		if configuration.UnverifiedExtraIndexURL != "" {
			arguments = append(arguments, "--unverified-extra-index-url", configuration.UnverifiedExtraIndexURL)
		}
	}
	if configuration.AllowTrustRemoteCode {
		arguments = append(arguments, "--allow-trust-remote-code", "true")
	}
	if configuration.AllowExternalTools {
		arguments = append(arguments, "--allow-external-tools", "true")
	}
	if configuration.AllowDynamicLoRA {
		arguments = append(arguments, "--allow-dynamic-lora", "true")
	}
	command := exec.Command(binaryPath, arguments...)
	processcontrol.Prepare(command, processcontrol.Options{HideWindow: true})
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	logs := newBoundedLog(maximumRuntimeLogBytes)
	command.Stderr = logs
	if err := command.Start(); err != nil {
		return nil, err
	}
	client := &Client{command: command, input: input, pending: map[uint64]chan protocolResponse{}, done: make(chan struct{}), logs: logs}
	go client.readResponses(output)
	handshakeContext, cancel := context.WithTimeout(ctx, companionHandshakeTimeout)
	defer cancel()
	var handshake Handshake
	if err := client.call(handshakeContext, "handshake", nil, &handshake); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("vLLM companion handshake failed: %w", err)
	}
	if handshake.Protocol != ProtocolVersion {
		_ = client.Close()
		return nil, fmt.Errorf("vLLM companion protocol %d is incompatible with required protocol %d", handshake.Protocol, ProtocolVersion)
	}
	for _, capability := range []string{"persistent_jobs", "atomic_environments", "generation", "pooling", "speech", "unix_socket"} {
		if !containsString(handshake.Capabilities, capability) {
			_ = client.Close()
			return nil, fmt.Errorf("vLLM companion lacks required capability %q", capability)
		}
	}
	client.state = handshake.State
	return client, nil
}

func (client *Client) State(ctx context.Context) State {
	var state State
	if err := client.call(ctx, "state", nil, &state); err != nil {
		client.stateMu.Lock()
		state = client.state
		client.stateMu.Unlock()
		state.LifecycleState = LifecycleFailed
		state.Error = errorText(sanitizeError(err))
		state.Retryable = true
		return state
	}
	client.stateMu.Lock()
	client.state = state
	client.stateMu.Unlock()
	return state
}

func (client *Client) StartInitialization(ctx context.Context, request InitRequest) (InitializationJob, error) {
	var job InitializationJob
	err := client.call(ctx, "initialize", request, &job)
	return job, err
}

func (client *Client) CancelInitialization(ctx context.Context) (InitializationJob, error) {
	var job InitializationJob
	err := client.call(ctx, "cancel_initialization", nil, &job)
	return job, err
}

func (client *Client) Load(ctx context.Context, request RuntimeLoadRequest) (RuntimeStatus, error) {
	var status RuntimeStatus
	err := client.call(ctx, "load", request, &status)
	return status, err
}

func (client *Client) Restart(ctx context.Context, kind RuntimeKind) (RuntimeStatus, error) {
	var status RuntimeStatus
	err := client.call(ctx, "restart", runtimeCall{Kind: kind}, &status)
	return status, err
}

func (client *Client) Unload(ctx context.Context, kind RuntimeKind) error {
	return client.call(ctx, "unload", runtimeCall{Kind: kind}, nil)
}

func (client *Client) Runtime(ctx context.Context, kind RuntimeKind) (RuntimeStatus, error) {
	var status RuntimeStatus
	err := client.call(ctx, "runtime", runtimeCall{Kind: kind}, &status)
	return status, err
}

func (client *Client) Close() error {
	client.closeOnce.Do(func() {
		inputError := client.input.Close()
		timer := time.NewTimer(companionShutdownTimeout)
		defer timer.Stop()
		select {
		case <-client.done:
		case <-timer.C:
			if err := client.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				inputError = errors.Join(inputError, err)
			}
			<-client.done
		}
		client.stateMu.Lock()
		client.waitError = errors.Join(client.waitError, inputError)
		client.stateMu.Unlock()
	})
	client.stateMu.Lock()
	defer client.stateMu.Unlock()
	return client.waitError
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
		return fmt.Errorf("write vLLM companion request: %w", err)
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
			return fmt.Errorf("decode vLLM companion response: %w", err)
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
	message := "vLLM companion terminated"
	if detail := client.logs.String(); detail != "" {
		message += ": " + detail
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

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
