package inventory

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"tensors-router/internal/cluster"
)

const (
	RoleUnknown    = "unknown"
	RoleLLM        = "llm"
	RoleImage      = "image"
	RoleEmbeddings = "embeddings"
	RoleMultimodal = "multimodal"
	RoleVAE        = "vae"
	RoleClip       = "clip"
	RoleT5         = "t5"
	RoleUpscaler   = "upscaler"
	RoleLoRA       = "lora"
	RoleVoice      = "voice"
	RoleMusic      = "music"
)

type FileRecord struct {
	Path         string   `json:"path"`
	Basename     string   `json:"basename"`
	Extension    string   `json:"extension"`
	Size         int64    `json:"size"`
	Modified     int64    `json:"modified"`
	NodeID       string   `json:"node_id"`
	Role         string   `json:"role"`
	Roles        []string `json:"roles"`
	ReferencedBy []string `json:"referenced_by,omitempty"`
}

type pathReference struct {
	role  string
	model string
}

func Scan(roots []string, models []cluster.Model, nodeID string) ([]FileRecord, error) {
	references := referencesByPath(models)
	files := make([]FileRecord, 0)
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		rootFiles, err := scanRoot(root, references, nodeID)
		if err != nil {
			return nil, err
		}
		files = append(files, rootFiles...)
	}
	sort.Slice(files, func(left, right int) bool {
		return files[left].Path < files[right].Path
	})
	return files, nil
}

func scanRoot(root string, references map[string][]pathReference, nodeID string) ([]FileRecord, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(absoluteRoot); err != nil {
		if os.IsNotExist(err) {
			return []FileRecord{}, nil
		}
		return nil, err
	}
	resolvedRoot, err := resolveExistingPath(absoluteRoot)
	if err != nil {
		return nil, err
	}
	files := make([]FileRecord, 0)
	err = filepath.WalkDir(resolvedRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return scanSymlink(path, resolvedRoot, references, nodeID, &files)
		}
		if entry.IsDir() {
			return nil
		}
		if !allowedExtension(filepath.Ext(path)) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, fileRecord(path, info, references, nodeID))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func scanSymlink(path string, root string, references map[string][]pathReference, nodeID string, files *[]FileRecord) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil
	}
	if !insideRoot(root, resolved) {
		return nil
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil
	}
	if info.IsDir() || !allowedExtension(filepath.Ext(path)) {
		return nil
	}
	*files = append(*files, fileRecord(path, info, references, nodeID))
	return nil
}

func fileRecord(path string, info os.FileInfo, references map[string][]pathReference, nodeID string) FileRecord {
	absolutePath, err := filepath.Abs(path)
	if err == nil {
		path = absolutePath
	}
	extension := strings.ToLower(filepath.Ext(path))
	roles, referencedBy := inferredRoles(path, extension, references)
	return FileRecord{
		Path:         filepath.Clean(path),
		Basename:     filepath.Base(path),
		Extension:    extension,
		Size:         info.Size(),
		Modified:     info.ModTime().Unix(),
		NodeID:       nodeID,
		Role:         roles[0],
		Roles:        roles,
		ReferencedBy: referencedBy,
	}
}

func inferredRoles(path string, extension string, references map[string][]pathReference) ([]string, []string) {
	roleSet := map[string]struct{}{}
	modelSet := map[string]struct{}{}
	for _, key := range pathKeys(path) {
		for _, reference := range references[key] {
			roleSet[reference.role] = struct{}{}
			modelSet[reference.model] = struct{}{}
		}
	}
	roles := sortedKeys(roleSet)
	if len(roles) == 0 {
		roles = []string{roleFromExtension(extension)}
	}
	referencedBy := sortedKeys(modelSet)
	return roles, referencedBy
}

func referencesByPath(models []cluster.Model) map[string][]pathReference {
	references := map[string][]pathReference{}
	for _, model := range models {
		modelName := model.PublicID
		if modelName == "" {
			modelName = model.LocalID
		}
		if model.Capabilities.Embeddings != nil {
			addReference(references, model.Capabilities.Embeddings.Model, RoleEmbeddings, modelName)
		}
		if model.Capabilities.Multimodal != nil {
			addReference(references, model.Capabilities.Multimodal.Projector, RoleMultimodal, modelName)
		}
		if model.Capabilities.Image != nil {
			addReference(references, model.Capabilities.Image.Model, RoleImage, modelName)
			addReference(references, model.Capabilities.Image.VAE, RoleVAE, modelName)
			addReference(references, model.Capabilities.Image.Clip1, RoleClip, modelName)
			addReference(references, model.Capabilities.Image.Clip2, RoleClip, modelName)
			addReference(references, model.Capabilities.Image.ClipL, RoleClip, modelName)
			addReference(references, model.Capabilities.Image.ClipG, RoleClip, modelName)
			addReference(references, model.Capabilities.Image.T5XXL, RoleT5, modelName)
			addReference(references, model.Capabilities.Image.Upscaler, RoleUpscaler, modelName)
			for _, lora := range model.Capabilities.Image.LoRA {
				addReference(references, lora, RoleLoRA, modelName)
			}
		}
		if model.Capabilities.Voice != nil {
			addReference(references, model.Capabilities.Voice.WhisperModel, RoleVoice, modelName)
			addReference(references, model.Capabilities.Voice.TTSModel, RoleVoice, modelName)
			addReference(references, model.Capabilities.Voice.WAVTokenizer, RoleVoice, modelName)
			addReference(references, model.Capabilities.Voice.Directory, RoleVoice, modelName)
		}
		if model.Capabilities.Music != nil {
			addReference(references, model.Capabilities.Music.LLM, RoleMusic, modelName)
			addReference(references, model.Capabilities.Music.Embeddings, RoleMusic, modelName)
			addReference(references, model.Capabilities.Music.Diffusion, RoleMusic, modelName)
			addReference(references, model.Capabilities.Music.VAE, RoleMusic, modelName)
		}
	}
	return references
}

func addReference(references map[string][]pathReference, path string, role string, model string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	for _, key := range pathKeys(path) {
		references[key] = append(references[key], pathReference{role: role, model: model})
	}
}

func pathKeys(path string) []string {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(path)))
	keys := []string{pathKey(clean), pathKey(filepath.ToSlash(clean))}
	if absolute, err := filepath.Abs(clean); err == nil {
		keys = append(keys, pathKey(absolute), pathKey(filepath.ToSlash(absolute)))
	}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		keys = append(keys, pathKey(resolved), pathKey(filepath.ToSlash(resolved)))
	}
	return uniqueStrings(keys)
}

func pathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

func resolveExistingPath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	info, statErr := os.Lstat(path)
	if statErr == nil && info.Mode()&os.ModeSymlink == 0 {
		absolute, absErr := filepath.Abs(path)
		if absErr != nil {
			return "", absErr
		}
		return absolute, nil
	}
	return "", err
}

func insideRoot(root string, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".." && !filepath.IsAbs(relative))
}

func roleFromExtension(extension string) string {
	switch strings.ToLower(extension) {
	case ".safetensors", ".ckpt":
		return RoleImage
	case ".gguf", ".bin":
		return RoleLLM
	case ".pt", ".pth", ".onnx":
		return RoleUpscaler
	default:
		return RoleUnknown
	}
}

func allowedExtension(extension string) bool {
	switch strings.ToLower(extension) {
	case ".gguf", ".bin", ".safetensors", ".ckpt", ".pt", ".pth", ".onnx":
		return true
	default:
		return false
	}
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func RootError(root string, err error) error {
	return fmt.Errorf("scan %q failed: %w", root, err)
}
