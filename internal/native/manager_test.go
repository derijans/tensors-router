package native

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
