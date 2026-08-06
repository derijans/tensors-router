package loadcapture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Asset struct {
	Role     string `json:"role"`
	Position int    `json:"position"`
	SHA256   string `json:"sha256"`
}

type Snapshot struct {
	SHA256     string
	JSON       []byte
	Assets     []Asset
	Redactions map[string]string
}

var modelFields = map[string]struct{}{
	"model": {}, "model_param": {}, "lora": {}, "mmproj": {}, "draftmodel": {},
	"sdmodel": {}, "sddiffusionmodel": {}, "sdhighnoisediffusionmodel": {}, "sdunconddiffusionmodel": {},
	"sdllm": {}, "sdllmvision": {}, "sdclipvision": {}, "sdipadapter": {}, "sdmotionmodule": {},
	"sdcontrolnet": {}, "sdpulidweights": {}, "sdpulididembedding": {}, "sdupscaler": {}, "sdvae": {},
	"sdt5xxl": {}, "sdclip1": {}, "sdclip2": {}, "sdclipl": {}, "sdclipg": {}, "sdlora": {},
	"whispermodel": {}, "whispercpp_vad_model": {}, "ttsmodel": {}, "ttswavtokenizer": {},
	"talkermodel": {}, "code2wavmodel": {}, "musicllm": {}, "musicembeddings": {}, "musicdiffusion": {},
	"musicvae": {}, "embeddingsmodel": {}, "ttsdir": {}, "models_dir": {}, "sdloramodeldir": {}, "sdhiresupscalersdir": {},
}

var filesystemFields = map[string]struct{}{
	"preloadstory": {}, "savedatafile": {}, "api_key_file": {}, "log_prompts_dir": {},
	"admindir": {}, "downloaddir": {}, "mcpfile": {}, "baseconfig": {},
}

func BuildSnapshot(configPath string) (Snapshot, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return Snapshot{}, err
	}
	values := map[string]any{}
	if err := json.Unmarshal(content, &values); err != nil {
		return Snapshot{}, err
	}
	builder := snapshotBuilder{configDir: filepath.Dir(configPath), redactions: make(map[string]string)}
	sanitized, err := builder.transformMap(values)
	if err != nil {
		return Snapshot{}, err
	}
	canonical, err := json.Marshal(sanitized)
	if err != nil {
		return Snapshot{}, err
	}
	sum := sha256.Sum256(canonical)
	return Snapshot{SHA256: hex.EncodeToString(sum[:]), JSON: canonical, Assets: builder.assets, Redactions: builder.redactions}, nil
}

type snapshotBuilder struct {
	configDir  string
	assets     []Asset
	redactions map[string]string
}

func (builder *snapshotBuilder) transformMap(values map[string]any) (map[string]any, error) {
	result := make(map[string]any, len(values))
	for key, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if isSecretField(normalized) || isLogicalModelField(normalized) || isOriginField(normalized) {
			result[key] = "[REDACTED]"
			continue
		}
		if _, ok := filesystemFields[normalized]; ok {
			if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
				result[key] = ""
			} else {
				result[key] = "[PATH]"
			}
			continue
		}
		if _, ok := modelFields[normalized]; ok {
			sanitized, err := builder.transformAssetValue(normalized, value, 0)
			if err != nil {
				return nil, err
			}
			result[key] = sanitized
			continue
		}
		result[key] = transformNonModelValue(value)
	}
	return result, nil
}

func (builder *snapshotBuilder) transformAssetValue(role string, value any, position int) (any, error) {
	switch typed := value.(type) {
	case string:
		value := strings.TrimSpace(typed)
		if value == "" {
			return "", nil
		}
		digest, variants, err := contentIdentity(builder.configDir, value)
		if err != nil {
			return nil, fmt.Errorf("verify load asset %s: %w", role, err)
		}
		identity := "sha256:" + digest
		for _, variant := range variants {
			builder.redactions[variant] = identity
		}
		builder.assets = append(builder.assets, Asset{Role: role, Position: position, SHA256: digest})
		return identity, nil
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			sanitized, err := builder.transformAssetValue(role, item, index)
			if err != nil {
				return nil, err
			}
			result[index] = sanitized
		}
		return result, nil
	case nil:
		return nil, nil
	default:
		return typed, nil
	}
}

func transformNonModelValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSecretField(strings.ToLower(strings.TrimSpace(key))) || isLogicalModelField(strings.ToLower(strings.TrimSpace(key))) || isOriginField(strings.ToLower(strings.TrimSpace(key))) {
				result[key] = "[REDACTED]"
			} else {
				result[key] = transformNonModelValue(item)
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = transformNonModelValue(item)
		}
		return result
	case string:
		if filesystemValue(typed) {
			return "[PATH]"
		}
		return typed
	default:
		return typed
	}
}

func contentIdentity(configDir string, value string) (string, []string, error) {
	path := value
	if !filepath.IsAbs(path) {
		path = filepath.Join(configDir, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		digest, err := hashDirectory(absolute)
		return digest, []string{value, absolute, filepath.Clean(absolute)}, err
	}
	digest, err := hashFile(absolute)
	return digest, []string{value, absolute, filepath.Clean(absolute)}, err
}

func hashDirectory(path string) (string, error) {
	hashes := []string{}
	err := filepath.WalkDir(path, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() {
			digest, err := hashFile(candidate)
			if err != nil {
				return err
			}
			hashes = append(hashes, digest)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(hashes)
	sum := sha256.Sum256([]byte(strings.Join(hashes, "\n")))
	return hex.EncodeToString(sum[:]), nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func isSecretField(key string) bool {
	return strings.Contains(key, "password") || strings.Contains(key, "secret") || strings.Contains(key, "token") || strings.Contains(key, "credential") || strings.Contains(key, "authorization") || strings.Contains(key, "apikey") || strings.Contains(key, "api_key") || key == "key" || strings.HasSuffix(key, "_key")
}

func isLogicalModelField(key string) bool {
	return strings.Contains(key, "modelname") || strings.Contains(key, "model_name") || strings.Contains(key, "model_id") || strings.HasPrefix(key, "horde")
}

func isOriginField(key string) bool {
	return strings.Contains(key, "repository") || strings.Contains(key, "huggingface") || strings.HasPrefix(key, "hf_")
}

func filesystemValue(value string) bool {
	value = strings.TrimSpace(value)
	return filepath.IsAbs(value) || strings.Contains(value, "/") || strings.Contains(value, "\\") || (len(value) > 2 && value[1] == ':')
}
