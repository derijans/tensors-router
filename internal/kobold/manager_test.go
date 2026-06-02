package kobold

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestLaunchArguments(t *testing.T) {
	manager, err := NewManager(ProcessConfig{
		BackendURL:   "http://127.0.0.1:6000",
		BinaryPath:   "./koboldcpp",
		ConfigDir:    "./kcpps",
		DataDir:      "./data",
		Multiuser:    3,
		ExtraArgs:    []string{"--quiet", "--routermode"},
		Quiet:        true,
		SkipLauncher: true,
		NoModel:      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	args := manager.LaunchArguments()
	expectSequence(t, args, "--host", "127.0.0.1")
	expectSequence(t, args, "--port", "6000")
	expectSequence(t, args, "--admindir", "./kcpps")
	expectSequence(t, args, "--multiuser", "3")
	expectPresent(t, args, "--admin")
	expectAbsent(t, args, "--routermode")
	expectPresent(t, args, "--nomodel")
	expectPresent(t, args, "--skiplauncher")
	expectPresent(t, args, "--quiet")
}

func TestReloadConfigUsesAdminEndpoint(t *testing.T) {
	var reloaded string
	var sawAuthorization bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/reload_config":
			sawAuthorization = r.Header.Get("Authorization") != ""
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			reloaded = payload["filename"]
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/api/extra/version":
			_, _ = w.Write([]byte(`{"result":"ok"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	manager, err := NewManager(ProcessConfig{
		BackendURL: server.URL,
		BinaryPath: "./koboldcpp",
		ConfigDir:  "./kcpps",
		DataDir:    "./data",
		Multiuser:  1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := manager.ReloadConfig(context.Background(), "a.kcpps"); err != nil {
		t.Fatal(err)
	}
	if reloaded != "a.kcpps" {
		t.Fatalf("unexpected reload filename %q", reloaded)
	}
	if !sawAuthorization {
		t.Fatalf("expected authorization header")
	}
}

func TestStartStopsUnhealthyManagedProcessBeforeReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process interrupt behavior differs on Windows")
	}

	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	manager, err := NewManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:1",
		BinaryPath: filepathThatShouldNotExist(t),
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		Multiuser:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.client.Timeout = 50 * time.Millisecond
	manager.cmd = cmd
	manager.waitDone = waitDone

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := manager.Start(ctx); err == nil {
		t.Fatalf("expected replacement start to fail")
	}
	if manager.cmd != nil {
		t.Fatalf("expected stale command to be cleared")
	}

	select {
	case <-waitDone:
	case <-time.After(3 * time.Second):
		t.Fatalf("managed process was not stopped")
	}
}

func expectSequence(t *testing.T, args []string, key string, value string) {
	t.Helper()
	for index := 0; index < len(args)-1; index++ {
		if args[index] == key && args[index+1] == value {
			return
		}
	}
	t.Fatalf("expected %s %s in %#v", key, value, args)
}

func filepathThatShouldNotExist(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/missing-koboldcpp"
}

func expectPresent(t *testing.T, args []string, key string) {
	t.Helper()
	for _, arg := range args {
		if arg == key {
			return
		}
	}
	t.Fatalf("expected %s in %#v", key, args)
}

func expectAbsent(t *testing.T, args []string, key string) {
	t.Helper()
	for _, arg := range args {
		if arg == key {
			t.Fatalf("did not expect %s in %#v", key, args)
		}
	}
}
