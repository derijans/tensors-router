package downloader

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestClientPerformsHandshakeAndReportsCompanionCrash(t *testing.T) {
	directory := t.TempDir()
	binaryName := "tensor-router-downloader"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(directory, binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "../../cmd/tensor-router-downloader")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build companion: %v\n%s", err, output)
	}
	configPath := filepath.Join(directory, "downloader.yaml")
	if err := os.WriteFile(configPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := StartClient(context.Background(), binaryPath, configPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	capability := client.Capability()
	if !capability.Available || !capability.Configured {
		t.Fatalf("unexpected companion capability %#v", capability)
	}
	if err := client.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-client.done:
	case <-time.After(5 * time.Second):
		t.Fatal("companion did not terminate")
	}
	capability = client.Capability()
	if capability.Available || !strings.Contains(capability.Reason, "companion terminated") {
		t.Fatalf("unexpected crashed companion capability %#v", capability)
	}
}

func TestClientCloseKillsUnresponsiveCompanion(t *testing.T) {
	if os.Getenv("TENSORS_ROUTER_UNRESPONSIVE_COMPANION") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	command := exec.Command(os.Args[0], "-test.run=^TestClientCloseKillsUnresponsiveCompanion$")
	command.Env = append(os.Environ(), "TENSORS_ROUTER_UNRESPONSIVE_COMPANION=1")
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := &boundedBuffer{limit: 16 << 10}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	client := &Client{command: command, input: input, pending: map[uint64]chan protocolResponse{}, done: make(chan struct{}), stderr: stderr}
	go client.readResponses(output)
	started := time.Now()
	closeError := client.Close()
	if closeError == nil || !strings.Contains(closeError.Error(), "did not exit within") {
		t.Fatalf("unexpected close result %v", closeError)
	}
	maximumCloseDuration := companionShutdownTimeout + companionKillWaitTimeout + 2*time.Second
	if elapsed := time.Since(started); elapsed > maximumCloseDuration {
		t.Fatalf("client close took %s, expected at most %s", elapsed, maximumCloseDuration)
	}
	select {
	case <-client.done:
	default:
		t.Fatal("forced companion termination did not reap the process")
	}
}
