//go:build !windows

package webui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"tensors-router/internal/processcontrol"
)

func TestRouterProcessKillAllowsGracefulExit(t *testing.T) {
	binaryPath := buildFakeRouterProcess(t)
	dir := t.TempDir()
	readyPath := filepath.Join(dir, "ready")
	shutdownPath := filepath.Join(dir, "shutdown")
	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(),
		"FAKE_ROUTER_READY_FILE="+readyPath,
		"FAKE_ROUTER_SHUTDOWN_FILE="+shutdownPath,
	)
	if err := processcontrol.Start(cmd, processcontrol.Options{}); err != nil {
		t.Fatal(err)
	}
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
		close(waitDone)
	}()
	t.Cleanup(func() {
		_ = processcontrol.Kill(cmd)
	})

	waitForFile(t, readyPath)
	process := &RouterProcess{
		managed:  true,
		cmd:      cmd,
		waitDone: waitDone,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := process.Kill(ctx); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, shutdownPath)
}

func buildFakeRouterProcess(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(sourcePath, []byte(fakeRouterProcessSource), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(dir, "fake-router-process")
	cmd := exec.Command("go", "build", "-o", outputPath, sourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fake router build failed: %v\n%s", err, string(output))
	}
	return outputPath
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("file was not written: %s", path)
}

const fakeRouterProcessSource = `package main

import (
	"os"
	"os/signal"
)

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	if path := os.Getenv("FAKE_ROUTER_READY_FILE"); path != "" {
		_ = os.WriteFile(path, []byte("ready"), 0o644)
	}
	<-signals
	if path := os.Getenv("FAKE_ROUTER_SHUTDOWN_FILE"); path != "" {
		_ = os.WriteFile(path, []byte("shutdown"), 0o644)
	}
}
`
