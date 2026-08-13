package vllm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type processReadyWriter struct {
	once  sync.Once
	ready chan struct{}
}

func (writer *processReadyWriter) Write(content []byte) (int, error) {
	writer.once.Do(func() { close(writer.ready) })
	return len(content), nil
}

func TestExecCommandRunnerStopsCommandOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	output := &processReadyWriter{ready: make(chan struct{})}
	done := make(chan error, 1)
	environment := append(os.Environ(), "TENSOR_ROUTER_VLLM_COMMAND_HELPER=1")
	go func() {
		done <- (ExecCommandRunner{}).Run(ctx, os.Args[0], []string{"-test.run=^TestExecCommandRunnerHelper$"}, environment, t.TempDir(), output)
	}()
	select {
	case <-output.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("helper command did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("command cancellation returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelled command did not stop")
	}
}

func TestExecCommandRunnerHelper(t *testing.T) {
	if os.Getenv("TENSOR_ROUTER_VLLM_COMMAND_HELPER") != "1" {
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, "ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	for {
		time.Sleep(time.Second)
	}
}

func TestCopyRegularFileHonorsCancellationAndRemovesPartialDestination(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "runtime.oci.source")
	destinationPath := filepath.Join(directory, "runtime.oci")
	if err := os.WriteFile(sourcePath, []byte("authorized-image"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := copyRegularFile(ctx, sourcePath, destinationPath, 0o600)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled OCI copy returned %v", err)
	}
	if _, err := os.Stat(destinationPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled OCI copy left destination: %v", err)
	}
}
