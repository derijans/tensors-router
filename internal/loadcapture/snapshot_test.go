package loadcapture

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSnapshotUsesContentIdentityWithoutPathsOrSecrets(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	firstModel := filepath.Join(firstDir, "private-name.gguf")
	secondModel := filepath.Join(secondDir, "different-name.gguf")
	if err := os.WriteFile(firstModel, []byte("same model bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondModel, []byte("same model bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstConfig := filepath.Join(firstDir, "first.kcpps")
	secondConfig := filepath.Join(secondDir, "second.kcpps")
	firstContent, err := json.Marshal(map[string]any{"model_param": firstModel, "threads": 8, "password": "router-secret", "api_key_file": "C:/keys/token", "hordemodelname": "logical-model", "mcp_enabled": true})
	if err != nil {
		t.Fatal(err)
	}
	secondContent, err := json.Marshal(map[string]any{"model_param": secondModel, "threads": 8, "password": "router-secret", "api_key_file": "C:/keys/token", "hordemodelname": "logical-model", "mcp_enabled": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(firstConfig, firstContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondConfig, secondContent, 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := BuildSnapshot(firstConfig)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildSnapshot(secondConfig)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 || !bytes.Equal(first.JSON, second.JSON) {
		t.Fatalf("identical model bytes must yield the same snapshot: %s != %s", first.SHA256, second.SHA256)
	}
	rendered := string(first.JSON)
	for _, forbidden := range []string{firstModel, secondModel, "private-name", "different-name", "router-secret", "logical-model", "C:/keys/token"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("snapshot leaked %q: %s", forbidden, rendered)
		}
	}
	if len(first.Assets) != 1 || first.Assets[0].Role != "model_param" || !strings.Contains(rendered, `"mcp_enabled":true`) {
		t.Fatalf("unexpected snapshot assets: %#v", first.Assets)
	}
}
func TestBuildSnapshotDirectoryIdentityIgnoresNamesAndTracksContent(t *testing.T) {
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	firstAssets := filepath.Join(firstRoot, "voice-assets")
	secondAssets := filepath.Join(secondRoot, "renamed-assets")
	if err := os.MkdirAll(firstAssets, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secondAssets, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(firstAssets, "a.gguf"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(firstAssets, "b.gguf"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondAssets, "renamed-two.gguf"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secondAssets, "renamed-one.gguf"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstConfig := filepath.Join(firstRoot, "first.kcpps")
	secondConfig := filepath.Join(secondRoot, "second.kcpps")
	firstJSON, _ := json.Marshal(map[string]any{"ttsdir": firstAssets})
	secondJSON, _ := json.Marshal(map[string]any{"ttsdir": secondAssets})
	if err := os.WriteFile(firstConfig, firstJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondConfig, secondJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := BuildSnapshot(firstConfig)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildSnapshot(secondConfig)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 {
		t.Fatalf("directory names changed identity: %s != %s", first.SHA256, second.SHA256)
	}
	if err := os.WriteFile(filepath.Join(secondAssets, "renamed-one.gguf"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := BuildSnapshot(secondConfig)
	if err != nil {
		t.Fatal(err)
	}
	if changed.SHA256 == second.SHA256 {
		t.Fatal("changed directory content retained the old identity")
	}
}

func TestBuildSnapshotRetainsAssetArrayRoleAndOrder(t *testing.T) {
	dir := t.TempDir()
	firstModel := filepath.Join(dir, "first.gguf")
	secondModel := filepath.Join(dir, "second.gguf")
	if err := os.WriteFile(firstModel, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondModel, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "array.kcpps")
	content, err := json.Marshal(map[string]any{"lora": []string{secondModel, firstModel}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := BuildSnapshot(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Assets) != 2 || snapshot.Assets[0].Role != "lora" || snapshot.Assets[0].Position != 0 || snapshot.Assets[1].Position != 1 {
		t.Fatalf("asset roles or positions changed: %#v", snapshot.Assets)
	}
	var sanitized map[string][]string
	if err := json.Unmarshal(snapshot.JSON, &sanitized); err != nil {
		t.Fatal(err)
	}
	if len(sanitized["lora"]) != 2 || sanitized["lora"][0] != "sha256:"+snapshot.Assets[0].SHA256 || sanitized["lora"][1] != "sha256:"+snapshot.Assets[1].SHA256 {
		t.Fatalf("asset array order changed: %s", snapshot.JSON)
	}
}
