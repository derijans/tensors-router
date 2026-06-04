package native

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestLlamaLaunchArgumentsFromKcpps(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "combo.kcpps"), []byte(`{
		"model_param":"C:/models/text.gguf",
		"contextsize":8192,
		"threads":12,
		"batchsize":512,
		"gpulayers":-1,
		"splitmode":"layer",
		"tensor_split":[1,2],
		"maingpu":1,
		"usemmap":false,
		"usemlock":true,
		"quantkv":"f16",
		"mmproj":"C:/models/mmproj.gguf",
		"mmprojcpu":true,
		"visionmintokens":32,
		"visionmaxtokens":512
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager, err := NewLlamaManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:6002",
		BinaryPath: "llama-server",
		ConfigDir:  dir,
		DataDir:    t.TempDir(),
		ExtraArgs:  []string{"--parallel", "2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	args, err := manager.LaunchArguments("combo.kcpps")
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"--host", "127.0.0.1",
		"--port", "6002",
		"--model", "C:/models/text.gguf",
		"--alias", "combo",
		"--ctx-size", "8192",
		"--threads", "12",
		"--batch-size", "512",
		"--n-gpu-layers", "-1",
		"--split-mode", "layer",
		"--tensor-split", "1,2",
		"--main-gpu", "1",
		"--no-mmap",
		"--mlock",
		"--cache-type-k", "f16",
		"--cache-type-v", "f16",
		"--mmproj", "C:/models/mmproj.gguf",
		"--no-mmproj-offload",
		"--image-min-tokens", "32",
		"--image-max-tokens", "512",
		"--parallel", "2",
	}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("unexpected args %#v", args)
	}
}

func TestSDCPPLaunchArgumentsFromKcpps(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "image.kcpps"), []byte(`{
		"sdmodel":"C:/models/dream.safetensors",
		"sdvae":"C:/models/vae.safetensors",
		"sdt5xxl":"C:/models/t5.gguf",
		"sdclipl":"C:/models/clip-l.gguf",
		"sdclipg":"C:/models/clip-g.gguf",
		"sdupscaler":"C:/models/upscale.pth",
		"sdthreads":8,
		"sdflashattention":true,
		"sdoffloadcpu":true,
		"sdvaecpu":true,
		"sdtiledvae":768
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager, err := NewSDCPPManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:7861",
		BinaryPath: "sd-server",
		ConfigDir:  dir,
		DataDir:    t.TempDir(),
		ExtraArgs:  []string{"--verbose"},
	})
	if err != nil {
		t.Fatal(err)
	}
	args, err := manager.LaunchArguments("image.kcpps")
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"--listen-ip", "127.0.0.1",
		"--listen-port", "7861",
		"--model", "C:/models/dream.safetensors",
		"--vae", "C:/models/vae.safetensors",
		"--t5xxl", "C:/models/t5.gguf",
		"--clip_l", "C:/models/clip-l.gguf",
		"--clip_g", "C:/models/clip-g.gguf",
		"--upscale-model", "C:/models/upscale.pth",
		"--threads", "8",
		"--fa",
		"--offload-to-cpu",
		"--vae-on-cpu",
		"--vae-tiling",
		"--vae-tile-size", "768x768",
		"--verbose",
	}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("unexpected args %#v", args)
	}
}

func TestNativeManagersStartAndStopServerProcessesWithoutRealModels(t *testing.T) {
	binaryPath := buildFakeNativeServer(t)
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "text.kcpps"), []byte(`{"model_param":"C:/missing/text.gguf"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "image.kcpps"), []byte(`{"sdmodel":"C:/missing/image.safetensors"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	llamaManager, err := NewLlamaManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:" + freeTCPPort(t),
		BinaryPath: binaryPath,
		ConfigDir:  configDir,
		DataDir:    filepath.Join(t.TempDir(), "llama"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := llamaManager.ReloadConfig(ctx, "text.kcpps"); err != nil {
		t.Fatal(err)
	}
	if !llamaManager.Healthy(ctx) {
		t.Fatalf("expected llama fake server to be healthy")
	}
	if err := llamaManager.Unload(ctx); err != nil {
		t.Fatal(err)
	}

	sdcppManager, err := NewSDCPPManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:" + freeTCPPort(t),
		BinaryPath: binaryPath,
		ConfigDir:  configDir,
		DataDir:    filepath.Join(t.TempDir(), "sdcpp"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sdcppManager.ReloadConfig(ctx, "image.kcpps"); err != nil {
		t.Fatal(err)
	}
	if !sdcppManager.Healthy(ctx) {
		t.Fatalf("expected sdcpp fake server to be healthy")
	}
	if err := sdcppManager.Unload(ctx); err != nil {
		t.Fatal(err)
	}
}

func buildFakeNativeServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(sourcePath, []byte(fakeNativeServerSource), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(dir, "fake-native-server")
	if runtime.GOOS == "windows" {
		outputPath += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", outputPath, sourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fake native server build failed: %v\n%s", err, string(output))
	}
	return outputPath
}

func freeTCPPort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

const fakeNativeServerSource = `package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	host := argValue("--host", "--listen-ip")
	if host == "" {
		host = "127.0.0.1"
	}
	port := argValue("--port", "--listen-port")
	if port == "" {
		os.Exit(2)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	})
	mux.HandleFunc("/sdapi/v1/sd-models", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{})
	})

	server := &http.Server{
		Addr:    net.JoinHostPort(host, port),
		Handler: mux,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		os.Exit(1)
	}
}

func argValue(names ...string) string {
	for index := 1; index < len(os.Args)-1; index++ {
		for _, name := range names {
			if os.Args[index] == name {
				return os.Args[index+1]
			}
		}
	}
	return ""
}
`
