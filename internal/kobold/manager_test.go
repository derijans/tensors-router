package kobold

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"tensors-router/internal/portalloc"
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

func TestNewManagerRejectsNonLoopbackAndBindOverrides(t *testing.T) {
	if _, err := NewManager(ProcessConfig{BackendURL: "http://192.168.1.20:5001", Multiuser: 1}); err == nil {
		t.Fatal("expected non-loopback backend rejection")
	}
	if _, err := NewManager(ProcessConfig{BackendURL: "http://127.0.0.1:5001", Multiuser: 1, ExtraArgs: []string{"--host=0.0.0.0"}}); err == nil {
		t.Fatal("expected bind override rejection")
	}
	if _, err := NewManager(ProcessConfig{BackendURL: "http://127.0.0.1", Multiuser: 1}); err == nil {
		t.Fatal("expected a backend URL with no port to be rejected")
	}
}

func TestDynamicPortReservedBeforeLaunchArguments(t *testing.T) {
	manager, err := NewManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:0",
		BinaryPath: "./koboldcpp",
		ConfigDir:  "./kcpps",
		DataDir:    "./data",
		Multiuser:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !manager.endpoint.Dynamic() {
		t.Fatal("expected a dynamic endpoint for port 0")
	}
	expectSequence(t, manager.LaunchArguments(), "--port", "0")

	if err := manager.endpoint.Reserve(portalloc.Default()); err != nil {
		t.Fatal(err)
	}
	_, port := manager.endpoint.HostPort()
	if port == "0" || port == "" {
		t.Fatalf("expected a reserved port, got %q", port)
	}
	expectSequence(t, manager.LaunchArguments(), "--port", port)
	manager.endpoint.Release(portalloc.Default())
}

func TestTwoDynamicManagersReserveDistinctPorts(t *testing.T) {
	first, err := NewManager(ProcessConfig{BackendURL: "http://127.0.0.1:0", Multiuser: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewEmbeddingsManager(ProcessConfig{BackendURL: "http://127.0.0.1:0", Multiuser: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.endpoint.Reserve(portalloc.Default()); err != nil {
		t.Fatal(err)
	}
	if err := second.endpoint.Reserve(portalloc.Default()); err != nil {
		t.Fatal(err)
	}
	_, firstPort := first.endpoint.HostPort()
	_, secondPort := second.endpoint.HostPort()
	if firstPort == secondPort {
		t.Fatalf("expected distinct dynamic ports, both got %s", firstPort)
	}
	first.endpoint.Release(portalloc.Default())
	second.endpoint.Release(portalloc.Default())
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

func TestReloadConfigPreservesMixedCaseFilename(t *testing.T) {
	filename := "Gemma4-31B-Nothhink.kcpps"
	var reloaded string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/admin/reload_config":
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

	manager, err := NewManager(ProcessConfig{BackendURL: server.URL, ConfigDir: filepath.Join(t.TempDir(), "MixedCase", "Configs"), Multiuser: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ReloadConfig(context.Background(), filename); err != nil {
		t.Fatal(err)
	}
	if reloaded != filename {
		t.Fatalf("unexpected reload filename %q", reloaded)
	}
}

func TestRoleSpecificRuntimeConfigurations(t *testing.T) {
	dir := t.TempDir()
	filename := "mixed.kcpps"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(`{
		"model_param":"C:/models/text.gguf",
		"sdmodel":"C:/models/image.safetensors",
		"ttsmodel":"C:/models/voice.gguf",
		"embeddingsmodel":"C:/models/embed.gguf",
		"embeddingsmaxctx":2048,
		"embeddingsgpu":false,
		"run_embed_separate":true,
		"usecuda":["normal","0"],
		"tensor_split":[1,2],
		"maingpu":1
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gpu.kcpps"), []byte(`{
		"embeddingsmodel":"C:/models/embed-gpu.gguf",
		"embeddingsgpu":true,
		"run_embed_separate":true,
		"usecuda":["normal","0"],
		"tensor_split":[1,2],
		"maingpu":1
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	primary, err := NewManager(ProcessConfig{BackendURL: "http://127.0.0.1:5001", ConfigDir: dir, Multiuser: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, primaryPath, _, err := primary.runtimeConfig(filename)
	if err != nil {
		t.Fatal(err)
	}
	primaryValues := readRuntimeConfigValues(t, primaryPath)
	if _, ok := primaryValues["embeddingsmodel"]; ok {
		t.Fatalf("primary config retained embedding model %#v", primaryValues)
	}
	if string(primaryValues["model_param"]) != `"C:/models/text.gguf"` || string(primaryValues["sdmodel"]) != `"C:/models/image.safetensors"` {
		t.Fatalf("primary config lost unrelated components %#v", primaryValues)
	}

	embeddings, err := NewEmbeddingsManager(ProcessConfig{BackendURL: "http://127.0.0.1:5004", ConfigDir: dir, Multiuser: 1, ExtraArgs: []string{"--usecuda", "--gpulayers", "99", "--quiet"}})
	if err != nil {
		t.Fatal(err)
	}
	runtimeFilename, embeddingsPath, _, err := embeddings.runtimeConfig(filename)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(runtimeFilename) != ".router-runtime" {
		t.Fatalf("unexpected private runtime filename %q", runtimeFilename)
	}
	embeddingValues := readRuntimeConfigValues(t, embeddingsPath)
	for _, forbidden := range []string{"model_param", "sdmodel", "ttsmodel", "usecuda", "tensor_split", "maingpu"} {
		if _, ok := embeddingValues[forbidden]; ok {
			t.Fatalf("embedding config retained %s: %#v", forbidden, embeddingValues)
		}
	}
	if string(embeddingValues["embeddingsmodel"]) != `"C:/models/embed.gguf"` || string(embeddingValues["usecpu"]) != "true" || string(embeddingValues["gpulayers"]) != "0" {
		t.Fatalf("embedding config is not CPU-isolated %#v", embeddingValues)
	}
	expectPresent(t, embeddings.LaunchArguments(), "--usecpu")
	expectAbsent(t, embeddings.LaunchArguments(), "--usecuda")
	expectAbsent(t, embeddings.LaunchArguments(), "--gpulayers")
	_, gpuPath, _, err := embeddings.runtimeConfig("gpu.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	gpuValues := readRuntimeConfigValues(t, gpuPath)
	if string(gpuValues["embeddingsgpu"]) != "true" || string(gpuValues["usecuda"]) != `["normal","0"]` || string(gpuValues["tensor_split"]) != "[1,2]" || string(gpuValues["maingpu"]) != "1" {
		t.Fatalf("GPU embedding config lost placement %#v", gpuValues)
	}
	expectAbsent(t, embeddings.LaunchArguments(), "--usecpu")
	expectPresent(t, embeddings.LaunchArguments(), "--usecuda")
	if err := embeddings.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(embeddingsPath); !os.IsNotExist(err) {
		t.Fatalf("generated config was not cleaned up: %v", err)
	}
}

func readRuntimeConfigValues(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	values := make(map[string]json.RawMessage)
	if err := json.Unmarshal(content, &values); err != nil {
		t.Fatal(err)
	}
	return values
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
	processExited := make(chan struct{})
	go func() {
		waitDone <- cmd.Wait()
		close(processExited)
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
	case <-processExited:
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
