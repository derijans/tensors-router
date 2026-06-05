package cook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tensors-router/internal/catalog"
)

type ModelCatalog interface {
	Resolve(id string) (catalog.Model, bool, error)
	ResolveImage(id string, activeConfigFilename string) (catalog.Model, bool, error)
}

type Writer struct {
	ConfigDir string
	FileRoots []string
	Catalog   ModelCatalog
	NodeID    string
	NodeURL   string
}

func (writer Writer) Preview(request NodeConfigRequest) (ConfigResult, error) {
	request.DryRun = true
	return writer.write(request)
}

func (writer Writer) Apply(request NodeConfigRequest) (ConfigResult, error) {
	request.DryRun = false
	return writer.write(request)
}

func (writer Writer) write(request NodeConfigRequest) (ConfigResult, error) {
	if strings.TrimSpace(writer.ConfigDir) == "" {
		return ConfigResult{}, fmt.Errorf("config dir is required")
	}
	if writer.Catalog == nil {
		return ConfigResult{}, fmt.Errorf("catalog is required")
	}
	id, err := SanitizedID(request.ID)
	if err != nil {
		return ConfigResult{}, err
	}
	components, err := normalizedComponents(request.Components)
	if err != nil {
		return ConfigResult{}, err
	}
	if reusable, result, err := writer.reusableConfig(components); err != nil {
		return ConfigResult{}, err
	} else if reusable {
		return result, nil
	}

	filename := id + ".kcpps"
	target := filepath.Join(writer.ConfigDir, filename)
	if filename != filepath.Base(filename) {
		return ConfigResult{}, fmt.Errorf("config filename is invalid")
	}
	exists := false
	if _, err := os.Stat(target); err == nil {
		exists = true
	} else if !os.IsNotExist(err) {
		return ConfigResult{}, err
	}
	if exists && !request.Overwrite {
		return ConfigResult{}, fmt.Errorf("config %q already exists", filename)
	}

	body, imagePath, err := writer.composedConfig(components)
	if err != nil {
		return ConfigResult{}, err
	}
	if !request.DryRun {
		if err := os.MkdirAll(writer.ConfigDir, 0o755); err != nil {
			return ConfigResult{}, err
		}
		content, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return ConfigResult{}, err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return ConfigResult{}, err
		}
	}

	result := ConfigResult{
		NodeID:         writer.NodeID,
		NodeURL:        writer.NodeURL,
		ModelID:        id,
		Filename:       filename,
		Kinds:          componentKinds(components),
		WouldOverwrite: exists,
	}
	if imagePath != "" {
		result.ImageID = id + "-" + filenameStem(imagePath)
	}
	return result, nil
}

func (writer Writer) reusableConfig(components []Component) (bool, ConfigResult, error) {
	configID := ""
	var model catalog.Model
	for _, component := range components {
		if componentSource(component) != SourceConfig {
			return false, ConfigResult{}, nil
		}
		selected, err := writer.resolveComponentModel(component)
		if err != nil {
			return false, ConfigResult{}, err
		}
		if configID == "" {
			configID = selected.ID
			model = selected
			continue
		}
		if selected.ID != configID {
			return false, ConfigResult{}, nil
		}
	}
	if configID == "" {
		return false, ConfigResult{}, nil
	}
	return true, ConfigResult{
		NodeID:   writer.NodeID,
		NodeURL:  writer.NodeURL,
		ModelID:  model.ID,
		ImageID:  model.ImageID,
		Filename: model.Filename,
		Kinds:    componentKinds(components),
		Reused:   true,
	}, nil
}

func (writer Writer) composedConfig(components []Component) (map[string]json.RawMessage, string, error) {
	body := map[string]json.RawMessage{}
	for _, component := range components {
		if componentSource(component) != SourceConfig {
			continue
		}
		source, err := writer.configBody(component)
		if err != nil {
			return nil, "", err
		}
		for key, value := range source {
			body[key] = value
		}
		break
	}

	imagePath := ""
	for _, component := range components {
		source := componentSource(component)
		switch component.Kind {
		case KindText:
			if source == SourceFile {
				if err := writer.validateRawFile(component.FilePath); err != nil {
					return nil, "", err
				}
				setJSONString(body, "model_param", component.FilePath)
				setJSONBool(body, "nomodel", false)
				continue
			}
			sourceBody, err := writer.configBody(component)
			if err != nil {
				return nil, "", err
			}
			copyKeys(body, sourceBody, textKeys)
			setJSONBool(body, "nomodel", false)
		case KindEmbeddings:
			if source == SourceFile {
				if err := writer.validateRawFile(component.FilePath); err != nil {
					return nil, "", err
				}
				setJSONString(body, "embeddingsmodel", component.FilePath)
				continue
			}
			sourceBody, err := writer.configBody(component)
			if err != nil {
				return nil, "", err
			}
			copyKeys(body, sourceBody, embeddingKeys)
		case KindImage:
			if source == SourceFile {
				if err := writer.validateRawFile(component.FilePath); err != nil {
					return nil, "", err
				}
				setJSONString(body, "sdmodel", component.FilePath)
				imagePath = component.FilePath
				continue
			}
			sourceBody, err := writer.configBody(component)
			if err != nil {
				return nil, "", err
			}
			copyPrefix(body, sourceBody, "sd")
			if raw := strings.TrimSpace(rawJSONString(sourceBody["sdmodel"])); raw != "" {
				imagePath = raw
			}
		default:
			return nil, "", fmt.Errorf("component kind %q is invalid", component.Kind)
		}
	}
	if !hasKind(components, KindText) {
		setJSONBool(body, "nomodel", true)
	}
	if imagePath == "" {
		imagePath = rawJSONString(body["sdmodel"])
	}
	return body, imagePath, nil
}

func (writer Writer) configBody(component Component) (map[string]json.RawMessage, error) {
	model, err := writer.resolveComponentModel(component)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(model.Path)
	if err != nil {
		return nil, err
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(content, &body); err != nil {
		return nil, err
	}
	return body, nil
}

func (writer Writer) resolveComponentModel(component Component) (catalog.Model, error) {
	switch component.Kind {
	case KindImage:
		imageID := strings.TrimSpace(component.ImageID)
		if imageID == "" {
			imageID = strings.TrimSpace(component.ModelID)
		}
		model, ok, err := writer.Catalog.ResolveImage(imageID, catalog.AllImageConfigs)
		if err != nil || ok {
			return model, err
		}
		return catalog.Model{}, fmt.Errorf("image model %q was not found", imageID)
	case KindText, KindEmbeddings:
		modelID := strings.TrimSpace(component.ModelID)
		if modelID == "" {
			return catalog.Model{}, fmt.Errorf("%s model id is required", component.Kind)
		}
		model, ok, err := writer.Catalog.Resolve(modelID)
		if err != nil || ok {
			return model, err
		}
		return catalog.Model{}, fmt.Errorf("model %q was not found", modelID)
	default:
		return catalog.Model{}, fmt.Errorf("component kind %q is invalid", component.Kind)
	}
}

func (writer Writer) validateRawFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("file path is required")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("file path must point to a file")
	}
	for _, root := range writer.FileRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		resolvedRoot, err := resolveExistingPath(absoluteRoot)
		if err != nil {
			return err
		}
		resolvedFile, err := resolveExistingPath(absolutePath)
		if err != nil {
			return err
		}
		if insideRoot(resolvedRoot, resolvedFile) {
			return nil
		}
	}
	return fmt.Errorf("file path is outside configured model roots")
}

func normalizedComponents(components []Component) ([]Component, error) {
	if len(components) == 0 {
		return nil, fmt.Errorf("at least one component is required")
	}
	result := make([]Component, 0, len(components))
	seen := map[string]struct{}{}
	for _, component := range components {
		component.Kind = strings.TrimSpace(component.Kind)
		component.Source = componentSource(component)
		component.NodeID = strings.TrimSpace(component.NodeID)
		component.NodeURL = strings.TrimSpace(component.NodeURL)
		component.ModelID = strings.TrimSpace(component.ModelID)
		component.ImageID = strings.TrimSpace(component.ImageID)
		component.FilePath = strings.TrimSpace(component.FilePath)
		if _, ok := seen[component.Kind]; ok {
			return nil, fmt.Errorf("duplicate %s component", component.Kind)
		}
		switch component.Kind {
		case KindText, KindImage, KindEmbeddings:
		default:
			return nil, fmt.Errorf("component kind %q is invalid", component.Kind)
		}
		if component.Source == SourceFile && component.FilePath == "" {
			return nil, fmt.Errorf("%s file path is required", component.Kind)
		}
		if component.Source == SourceConfig && component.ModelID == "" && component.ImageID == "" {
			return nil, fmt.Errorf("%s model id is required", component.Kind)
		}
		seen[component.Kind] = struct{}{}
		result = append(result, component)
	}
	return result, nil
}

func componentSource(component Component) string {
	source := strings.TrimSpace(component.Source)
	if source == "" {
		if strings.TrimSpace(component.FilePath) != "" {
			return SourceFile
		}
		return SourceConfig
	}
	return source
}

func componentKinds(components []Component) []string {
	kinds := make([]string, 0, len(components))
	for _, component := range components {
		kinds = append(kinds, component.Kind)
	}
	sort.Strings(kinds)
	return kinds
}

func hasKind(components []Component, kind string) bool {
	for _, component := range components {
		if component.Kind == kind {
			return true
		}
	}
	return false
}

func SanitizedID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("id is required")
	}
	var builder strings.Builder
	lastDash := false
	for _, char := range strings.ToLower(id) {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
			lastDash = false
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
			lastDash = false
		case char == '_' || char == '-':
			builder.WriteRune(char)
			lastDash = char == '-'
		default:
			if !lastDash {
				builder.WriteRune('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(builder.String(), "-_")
	if result == "" {
		return "", fmt.Errorf("id is invalid")
	}
	if result != filepath.Base(result) {
		return "", fmt.Errorf("id is invalid")
	}
	return result, nil
}

func copyKeys(dst map[string]json.RawMessage, src map[string]json.RawMessage, keys []string) {
	for _, key := range keys {
		if value, ok := src[key]; ok {
			dst[key] = value
		}
	}
}

func copyPrefix(dst map[string]json.RawMessage, src map[string]json.RawMessage, prefix string) {
	for key, value := range src {
		if strings.HasPrefix(key, prefix) {
			dst[key] = value
		}
	}
}

func setJSONString(body map[string]json.RawMessage, key string, value string) {
	content, _ := json.Marshal(value)
	body[key] = content
}

func setJSONBool(body map[string]json.RawMessage, key string, value bool) {
	content, _ := json.Marshal(value)
	body[key] = content
}

func rawJSONString(value json.RawMessage) string {
	if len(value) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

func filenameStem(value string) string {
	value = strings.TrimSpace(value)
	separator := strings.LastIndexAny(value, `/\`)
	if separator >= 0 {
		value = value[separator+1:]
	}
	extension := filepath.Ext(value)
	if extension == "" {
		return value
	}
	return strings.TrimSuffix(value, extension)
}

func insideRoot(root string, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".." && !filepath.IsAbs(relative))
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

var textKeys = []string{
	"model",
	"model_param",
	"threads",
	"batchsize",
	"gpulayers",
	"splitmode",
	"tensor_split",
	"maingpu",
	"usemmap",
	"usemlock",
	"quantkv",
	"contextsize",
	"mmproj",
	"mmprojcpu",
	"visionmaxres",
	"visionmintokens",
	"visionmaxtokens",
}

var embeddingKeys = []string{
	"embeddingsmodel",
	"embeddingsmaxctx",
	"embeddingsgpu",
}
