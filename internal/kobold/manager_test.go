package kobold

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLaunchArguments(t *testing.T) {
	manager, err := NewManager(ProcessConfig{
		BackendURL:   "http://127.0.0.1:6000",
		BinaryPath:   "./koboldcpp",
		ConfigDir:    "./kcpps",
		DataDir:      "./data",
		Multiuser:    3,
		ExtraArgs:    []string{"--quiet"},
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
	expectPresent(t, args, "--routermode")
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

func expectSequence(t *testing.T, args []string, key string, value string) {
	t.Helper()
	for index := 0; index < len(args)-1; index++ {
		if args[index] == key && args[index+1] == value {
			return
		}
	}
	t.Fatalf("expected %s %s in %#v", key, value, args)
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
