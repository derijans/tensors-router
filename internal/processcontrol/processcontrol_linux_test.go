//go:build linux

package processcontrol

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("PROCESSCONTROL_TEST_HELPER") == "parent-death" {
		runParentDeathHelper()
		return
	}
	os.Exit(m.Run())
}

func TestLinuxKillStopsProcessGroup(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	cmd := exec.Command("/bin/sh", "-c", `sleep 60 & echo $! > "$1"; wait`, "process-group", pidPath)
	Prepare(cmd, Options{})
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
		close(waitDone)
	}()
	t.Cleanup(func() {
		_ = Kill(cmd)
		waitForCommandExit(waitDone, time.Second)
	})

	childPID := waitForPIDFile(t, pidPath)
	if err := Kill(cmd); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatal(err)
	}
	if !waitForCommandExit(waitDone, 3*time.Second) {
		t.Fatalf("command did not exit")
	}
	waitForMissingProcess(t, childPID)
}

func TestLinuxParentDeathSignalStopsManagedChild(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	helper := exec.Command(os.Args[0], "-test.run", "TestLinuxParentDeathSignalStopsManagedChild")
	helper.Env = append(os.Environ(),
		"PROCESSCONTROL_TEST_HELPER=parent-death",
		"PROCESSCONTROL_CHILD_PID_FILE="+pidPath,
	)
	output, err := helper.CombinedOutput()
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, string(output))
	}
	childPID := waitForPIDFile(t, pidPath)
	waitForMissingProcess(t, childPID)
}

func runParentDeathHelper() {
	pidPath := os.Getenv("PROCESSCONTROL_CHILD_PID_FILE")
	cmd := exec.Command("/bin/sleep", "60")
	Prepare(cmd, Options{})
	if err := cmd.Start(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	os.Exit(0)
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(content)) != "" {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(content)))
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			return pid
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("pid file was not written: %s", path)
	return 0
}

func waitForMissingProcess(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processExists(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("process %d is still alive", pid)
}

func processExists(pid int) bool {
	if processIsZombie(pid) {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func processIsZombie(pid int) bool {
	content, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	index := strings.LastIndex(string(content), ") ")
	if index == -1 {
		return false
	}
	fields := strings.Fields(string(content[index+2:]))
	return len(fields) > 0 && fields[0] == "Z"
}

func waitForCommandExit(waitDone <-chan error, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-waitDone:
		return true
	case <-timer.C:
		return false
	}
}
