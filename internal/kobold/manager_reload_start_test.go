package kobold

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

const sleepingHelperEnv = "KOBOLD_TEST_SLEEPING_PROCESS"

func TestMain(m *testing.M) {
	if os.Getenv(sleepingHelperEnv) == "1" {
		time.Sleep(time.Minute)
		return
	}
	os.Exit(m.Run())
}

func startSleepingProcess(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), sleepingHelperEnv+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd
}

func managerOnProcessWithPort(t *testing.T, serverURL string, cmd *exec.Cmd) *Manager {
	t.Helper()
	manager, err := NewManager(ProcessConfig{
		BackendURL: serverURL,
		BinaryPath: filepathThatShouldNotExist(t),
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		Multiuser:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.probeClient.Timeout = 50 * time.Millisecond
	manager.cmd = cmd
	manager.exitDone = make(chan error)
	return manager
}

func serverHealthyAfter(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	readyAt := time.Now().Add(delay)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if time.Now().Before(readyAt) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestStartWaitsForAProcessThatIsReloadingInsteadOfReplacingIt(t *testing.T) {
	server := serverHealthyAfter(t, 300*time.Millisecond)
	cmd := startSleepingProcess(t)
	manager := managerOnProcessWithPort(t, server.URL, cmd)
	manager.reloadsActive.Add(1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start during a reload failed: %v", err)
	}

	if manager.cmd != cmd {
		t.Fatal("start replaced the process that was still coming up from a reload")
	}
	if cmd.ProcessState != nil {
		t.Fatal("start killed the process that was still coming up from a reload")
	}
}

func TestStartStillReplacesAnUnhealthyProcessWhenNoReloadIsRunning(t *testing.T) {
	server := serverHealthyAfter(t, time.Hour)
	cmd := startSleepingProcess(t)
	manager := managerOnProcessWithPort(t, server.URL, cmd)
	var stopped atomic.Bool
	manager.waitDone = make(chan error, 1)
	go func() {
		_ = cmd.Wait()
		stopped.Store(true)
		manager.waitDone <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := manager.Start(ctx); err == nil {
		t.Fatal("expected the replacement start to fail with no binary to launch")
	}

	deadline := time.Now().Add(5 * time.Second)
	for !stopped.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !stopped.Load() {
		t.Fatal("the unhealthy process was left running")
	}
}
