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
	first, err := BuildSnapshot(firstConfig, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildSnapshot(secondConfig, nil)
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
	first, err := BuildSnapshot(firstConfig, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildSnapshot(secondConfig, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 {
		t.Fatalf("directory names changed identity: %s != %s", first.SHA256, second.SHA256)
	}
	if err := os.WriteFile(filepath.Join(secondAssets, "renamed-one.gguf"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := BuildSnapshot(secondConfig, nil)
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
	snapshot, err := BuildSnapshot(configPath, nil)
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

func TestBuildSnapshotHashesSDCPPTokenizerAndAudioEncoder(t *testing.T) {
	dir := t.TempDir()
	tokenizer := filepath.Join(dir, "private-tokenizer.json")
	audioEncoder := filepath.Join(dir, "private-wav2vec2.gguf")
	if err := os.WriteFile(tokenizer, []byte("tokenizer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(audioEncoder, []byte("audio encoder"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "sdcpp.kcpps")
	content, err := json.Marshal(map[string]any{"sdtokenizer": tokenizer, "sdaudioencoder": audioEncoder})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := BuildSnapshot(configPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]bool{}
	for _, asset := range snapshot.Assets {
		roles[asset.Role] = true
	}
	if !roles["sdtokenizer"] || !roles["sdaudioencoder"] {
		t.Fatalf("sd.cpp tokenizer and audio encoder must be content-identified assets: %#v", snapshot.Assets)
	}
	for _, forbidden := range []string{"private-tokenizer", "private-wav2vec2"} {
		if strings.Contains(string(snapshot.JSON), forbidden) {
			t.Fatalf("snapshot leaked %q: %s", forbidden, snapshot.JSON)
		}
	}
}

func TestBuildSnapshotDirectoryIdentityIncludesSymlinkedShards(t *testing.T) {
	linkedRoot := t.TempDir()
	copiedRoot := t.TempDir()
	shardStore := t.TempDir()
	linkedAssets := filepath.Join(linkedRoot, "assets")
	copiedAssets := filepath.Join(copiedRoot, "assets")
	for _, dir := range []string{linkedAssets, copiedAssets} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "first.gguf"), []byte("first shard"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sharedShard := filepath.Join(shardStore, "second.gguf")
	if err := os.WriteFile(sharedShard, []byte("second shard"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sharedShard, filepath.Join(linkedAssets, "second.gguf")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(copiedAssets, "second.gguf"), []byte("second shard"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkedConfig := filepath.Join(linkedRoot, "linked.kcpps")
	copiedConfig := filepath.Join(copiedRoot, "copied.kcpps")
	for config, assets := range map[string]string{linkedConfig: linkedAssets, copiedConfig: copiedAssets} {
		content, err := json.Marshal(map[string]any{"ttsdir": assets})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(config, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	linked, err := BuildSnapshot(linkedConfig, nil)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := BuildSnapshot(copiedConfig, nil)
	if err != nil {
		t.Fatal(err)
	}

	if linked.SHA256 != copied.SHA256 {
		t.Fatalf("a symlinked shard must count toward the directory identity: %s != %s", linked.SHA256, copied.SHA256)
	}
}
