package modelassets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

var modelFields = map[string]struct{}{
	"model": {}, "model_param": {}, "mmproj": {}, "draftmodel": {}, "lora": {},
	"sdmodel": {}, "sddiffusionmodel": {}, "sdhighnoisediffusionmodel": {}, "sdunconddiffusionmodel": {},
	"sdupscaler": {}, "sdvae": {}, "sdaudiovae": {}, "sdt5xxl": {}, "sdclip1": {}, "sdclip2": {}, "sdclipl": {}, "sdclipg": {}, "sdclipvision": {},
	"sdphotomaker": {}, "sdlora": {}, "sdllm": {}, "sdllmvision": {}, "sdtokenizer": {}, "sdaudioencoder": {}, "sdembeddingsconnectors": {}, "sdcontrolnet": {}, "sdpulidweights": {}, "sdpulididembedding": {}, "sdipadapter": {}, "sdmotionmodule": {},
	"whispermodel": {}, "whispercpp_vad_model": {}, "ttsmodel": {}, "ttswavtokenizer": {}, "talkermodel": {}, "code2wavmodel": {},
	"musicllm": {}, "musicembeddings": {}, "musicdiffusion": {}, "musicvae": {}, "embeddingsmodel": {},
}

type Origin struct {
	Repository string
	Commit     string
	Path       string
}

func (origin Origin) URI() string {
	if !safeRepository(origin.Repository) || !validCommit(origin.Commit) || !safeRepositoryPath(origin.Path) {
		return ""
	}
	return "hf://" + origin.Repository + "@" + origin.Commit + "/" + origin.Path
}

type HashFile func(path string) (hash string, err error)
type FindOrigin func(hash string) (Origin, bool)
type FindPath func(hash string, filename string) (string, bool)

type Reference struct {
	Hash     string
	Filename string
	HF       string
}

type ResolveReference func(reference Reference) (string, bool)

type Resolution struct {
	Path         string
	Source       string
	Verification string
	Commit       string
}

type ResolveReferenceDetailed func(reference Reference) (Resolution, bool)

type FieldResult struct {
	Field        string `json:"field"`
	Hash         string `json:"hash"`
	Resolved     bool   `json:"resolved"`
	Failure      string `json:"failure,omitempty"`
	Source       string `json:"source,omitempty"`
	Verification string `json:"verification,omitempty"`
	Commit       string `json:"commit,omitempty"`
}

type ResolveResult struct {
	Content []byte        `json:"-"`
	Fields  []FieldResult `json:"fields"`
}

func Resolve(content []byte, findPath FindPath) (ResolveResult, error) {
	if findPath == nil {
		return ResolveResult{}, fmt.Errorf("asset lookup function is required")
	}
	return ResolveWith(content, func(reference Reference) (string, bool) { return findPath(reference.Hash, reference.Filename) })
}

func ResolveWith(content []byte, findPath ResolveReference) (ResolveResult, error) {
	if findPath == nil {
		return ResolveResult{}, fmt.Errorf("asset lookup function is required")
	}
	return ResolveDetailed(content, func(reference Reference) (Resolution, bool) {
		path, found := findPath(reference)
		return Resolution{Path: path}, found
	})
}

func HashBytes(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func ValidHash(value string) bool    { return validHash(value) }
func SafeFilename(value string) bool { return safeFilename(value) }
func IsModelField(value string) bool { return isModelField(value) }

func ParseHFURI(value string) (Origin, error) {
	if !safeHFURI(value) {
		return Origin{}, fmt.Errorf("invalid Hugging Face model URI")
	}
	rest := strings.TrimPrefix(value, "hf://")
	repository, location, _ := strings.Cut(rest, "@")
	commit, path, _ := strings.Cut(location, "/")
	return Origin{Repository: repository, Commit: commit, Path: path}, nil
}

func UnresolvedFields(content []byte) (int, error) {
	config, err := decodeConfig(content)
	if err != nil {
		return 0, err
	}
	if err := validateForms(config); err != nil {
		return 0, err
	}
	count := 0
	for key, value := range config {
		if strings.HasSuffix(key, "_hash") {
			values, _, _ := pathValues(value)
			count += len(values)
		}
	}
	return count, nil
}

func Substitute(content []byte, field string, position *int, expectedHash string, replacementHash string, replacementFilename string, origin Origin) ([]byte, error) {
	config, err := decodeConfig(content)
	if err != nil {
		return nil, err
	}
	if err := validateForms(config); err != nil {
		return nil, err
	}
	if !isModelField(field) || !validHash(expectedHash) || !validHash(replacementHash) || !safeFilename(replacementFilename) || origin.URI() == "" {
		return nil, fmt.Errorf("invalid model asset substitution")
	}
	hashKey := field + "_hash"
	hashes, array, ok := pathValues(config[hashKey])
	if !ok {
		return nil, fmt.Errorf("model field %q is not portable", field)
	}
	index := 0
	if array {
		if position == nil || *position < 0 || *position >= len(hashes) {
			return nil, fmt.Errorf("model field %q requires a valid array position", field)
		}
		index = *position
	} else if position != nil {
		return nil, fmt.Errorf("model field %q is not an array", field)
	}
	if hashes[index] != expectedHash {
		return nil, fmt.Errorf("model field %q changed before substitution", field)
	}
	filenames, _, _ := pathValues(config[field+"_filename"])
	hfValues := make([]any, len(hashes))
	if existing, found := config[field+"_hf"]; found {
		values, _, _ := nullableStringValues(existing)
		for valueIndex, value := range values {
			if value != "" {
				hfValues[valueIndex] = value
			}
		}
	}
	hashes[index] = replacementHash
	filenames[index] = replacementFilename
	hfValues[index] = origin.URI()
	config[hashKey] = scalarOrArray(hashes, array)
	config[field+"_filename"] = scalarOrArray(filenames, array)
	config[field+"_hf"] = scalarOrArray(hfValues, array)
	return json.MarshalIndent(config, "", "  ")
}

func decodeConfig(content []byte) (map[string]any, error) {
	var config map[string]any
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, fmt.Errorf("invalid KCPPS JSON: %w", err)
	}
	if config == nil {
		return nil, fmt.Errorf("KCPPS config must be an object")
	}
	return config, nil
}

func portableKey(key string) (string, string, bool) {
	for _, suffix := range []string{"_filename", "_hash", "_hf"} {
		if strings.HasSuffix(key, suffix) {
			return strings.TrimSuffix(key, suffix), suffix, true
		}
	}
	return "", "", false
}

func pathValues(value any) ([]string, bool, bool) {
	switch typed := value.(type) {
	case string:
		return []string{typed}, false, true
	case []any:
		values := make([]string, len(typed))
		for index, item := range typed {
			stringValue, ok := item.(string)
			if !ok {
				return nil, false, false
			}
			values[index] = stringValue
		}
		return values, true, true
	default:
		return nil, false, false
	}
}

func nullableStringValues(value any) ([]string, bool, bool) {
	if stringValue, ok := value.(string); ok {
		return []string{stringValue}, false, true
	}
	items, ok := value.([]any)
	if !ok {
		return nil, false, false
	}
	values := make([]string, len(items))
	for index, item := range items {
		if item == nil {
			continue
		}
		stringValue, ok := item.(string)
		if !ok {
			return nil, false, false
		}
		values[index] = stringValue
	}
	return values, true, true
}

func scalarOrArray[T any](values []T, array bool) any {
	if array {
		result := make([]any, len(values))
		for index := range values {
			result[index] = values[index]
		}
		return result
	}
	return values[0]
}

func anyNonNil(values []any) bool {
	for _, value := range values {
		if value != nil {
			return true
		}
	}
	return false
}

func isModelField(key string) bool {
	_, ok := modelFields[strings.ToLower(strings.TrimSpace(key))]
	return ok
}

func validHash(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func safeFilename(value string) bool {
	if value == "" || len(value) > 255 || value != filepath.Base(value) || !filepath.IsLocal(value) || value == "." || value == ".." || strings.ContainsAny(value, "\\/:\"<>|?*") || strings.HasSuffix(value, ".") || strings.HasSuffix(value, " ") {
		return false
	}
	for _, character := range value {
		if character < 32 || character == 127 {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(value, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return false
	}
	return true
}

func safeRepository(value string) bool {
	parts := strings.Split(value, "/")
	return len(parts) == 2 && safeSegment(parts[0]) && safeSegment(parts[1])
}

func safeRepositoryPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if !safeSegment(segment) {
			return false
		}
	}
	return true
}

func safeSegment(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	return !strings.ContainsAny(value, "\\/@?#")
}

func validCommit(value string) bool {
	if len(value) < 7 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func safeHFURI(value string) bool {
	if !strings.HasPrefix(value, "hf://") {
		return false
	}
	rest := strings.TrimPrefix(value, "hf://")
	repository, location, ok := strings.Cut(rest, "@")
	if !ok {
		return false
	}
	commit, path, ok := strings.Cut(location, "/")
	return ok && safeRepository(repository) && validCommit(commit) && safeRepositoryPath(path)
}
