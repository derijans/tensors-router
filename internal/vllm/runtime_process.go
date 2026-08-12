package vllm

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"tensors-router/internal/processcontrol"
)

const maximumRuntimeLogBytes = 64 << 10

type RuntimeLauncher interface {
	Start(context.Context, string, []string, []string, string, io.Writer) (RuntimeChild, error)
}

type RuntimeChild interface {
	Wait() error
	Stop(context.Context) error
}

type ExecRuntimeLauncher struct{}

func (ExecRuntimeLauncher) Start(_ context.Context, executable string, arguments []string, environment []string, directory string, output io.Writer) (RuntimeChild, error) {
	command := exec.Command(executable, arguments...)
	command.Env = environment
	command.Dir = directory
	command.Stdout = output
	command.Stderr = output
	if err := processcontrol.Start(command, processcontrol.Options{HideWindow: true}); err != nil {
		return nil, err
	}
	return &execRuntimeChild{command: command}, nil
}

type execRuntimeChild struct {
	command   *exec.Cmd
	done      chan struct{}
	waitStart sync.Once
	waitErr   error
}

func (child *execRuntimeChild) Wait() error {
	child.waitStart.Do(func() {
		child.done = make(chan struct{})
		go func() {
			child.waitErr = child.command.Wait()
			close(child.done)
		}()
	})
	<-child.done
	return child.waitErr
}

func (child *execRuntimeChild) Stop(ctx context.Context) error {
	if child.command.Process == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	err := processcontrol.Stop(ctx, child.command, done, 5*time.Second, 5*time.Second)
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

type boundedLog struct {
	mu    sync.Mutex
	data  []byte
	limit int
}

func newBoundedLog(limit int) *boundedLog {
	return &boundedLog{limit: limit}
}

func (log *boundedLog) Write(content []byte) (int, error) {
	log.mu.Lock()
	defer log.mu.Unlock()
	if len(content) >= log.limit {
		log.data = append(log.data[:0], content[len(content)-log.limit:]...)
		return len(content), nil
	}
	overflow := len(log.data) + len(content) - log.limit
	if overflow > 0 {
		copy(log.data, log.data[overflow:])
		log.data = log.data[:len(log.data)-overflow]
	}
	log.data = append(log.data, content...)
	return len(content), nil
}

func (log *boundedLog) String() string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return redactSensitive(strings.TrimSpace(string(log.data)))
}

func redactSensitive(value string) string {
	value = bearerCredentialPattern.ReplaceAllString(value, "$1 [REDACTED]")
	value = sensitiveAssignmentPattern.ReplaceAllString(value, "$1$2[REDACTED]")
	value = queryCredentialPattern.ReplaceAllString(value, "$1[REDACTED]")
	return urlCredentialPattern.ReplaceAllString(value, "$1[REDACTED]@")
}

var sensitiveAssignmentPattern = regexp.MustCompile(`(?i)["']?\b(authorization|proxy-authorization|api[_-]?key|hf[_-]?token|hugging[_-]?face[_-]?hub[_-]?token|access[_-]?token)["']?(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,&]+)`)
var bearerCredentialPattern = regexp.MustCompile(`(?i)\b(bearer)\s+[^\s,"']+`)
var queryCredentialPattern = regexp.MustCompile(`(?i)([?&](?:access[_-]?)?token=)[^&\s]+`)
var urlCredentialPattern = regexp.MustCompile(`(https?://)[^/@\s]+@`)

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
