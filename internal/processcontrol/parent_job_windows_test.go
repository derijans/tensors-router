//go:build windows

package processcontrol

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

const lifetimeChildEnvironment = "TENSORS_ROUTER_LIFETIME_CHILD"

func TestBackendDiesWhenItsParentProcessIsKilled(t *testing.T) {
	switch os.Getenv(lifetimeChildEnvironment) {
	case "parent":
		backend := exec.Command(os.Args[0], "-test.run=^TestBackendDiesWhenItsParentProcessIsKilled$")
		backend.Env = append(os.Environ(), lifetimeChildEnvironment+"=backend")
		if err := Start(backend, Options{TerminateWithParent: true}); err != nil {
			os.Exit(2)
		}
		_, _ = os.Stdout.WriteString(strconv.Itoa(backend.Process.Pid) + "\n")
		time.Sleep(time.Hour)
		return
	case "backend":
		time.Sleep(time.Hour)
		return
	}
	parent := exec.Command(os.Args[0], "-test.run=^TestBackendDiesWhenItsParentProcessIsKilled$")
	parent.Env = append(os.Environ(), lifetimeChildEnvironment+"=parent")
	output, err := parent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	backendPID := readPID(t, output)
	backend, err := os.FindProcess(backendPID)
	if err != nil {
		t.Fatal(err)
	}
	backendExited := make(chan struct{})
	go func() {
		_, _ = backend.Wait()
		close(backendExited)
	}()
	if err := parent.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = parent.Wait()
	select {
	case <-backendExited:
	case <-time.After(10 * time.Second):
		_ = backend.Kill()
		t.Fatal("backend process outlived its killed parent")
	}
}

func readPID(t *testing.T, output io.Reader) int {
	t.Helper()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil {
		t.Fatalf("parent did not report its backend: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("parent reported %q instead of a backend pid", line)
	}
	return pid
}
