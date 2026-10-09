package cook

import (
	"encoding/json"
	"fmt"
	"strings"
)

type componentComposer struct {
	fileOptionKey func(Component) string
	copySettings  func(body map[string]json.RawMessage, source map[string]json.RawMessage)
	enablesText   bool
}

func fixedOptionKey(key string) func(Component) string {
	return func(Component) string { return key }
}

func componentOptionKey(component Component) string {
	return component.OptionKey
}

func copyKeySet(keys []string) func(map[string]json.RawMessage, map[string]json.RawMessage) {
	return func(body map[string]json.RawMessage, source map[string]json.RawMessage) {
		copyKeys(body, source, keys)
	}
}

var componentComposers = map[string]componentComposer{
	KindText:       {fileOptionKey: fixedOptionKey("model_param"), copySettings: copyKeySet(textKeys), enablesText: true},
	KindEmbeddings: {fileOptionKey: fixedOptionKey("embeddingsmodel"), copySettings: copyKeySet(embeddingKeys)},
	KindImage: {fileOptionKey: fixedOptionKey("sdmodel"), copySettings: func(body map[string]json.RawMessage, source map[string]json.RawMessage) {
		copyPrefix(body, source, "sd")
	}},
	KindVoice: {fileOptionKey: componentOptionKey, copySettings: copyKeySet(voiceKeys)},
	KindMusic: {fileOptionKey: componentOptionKey, copySettings: copyKeySet(musicKeys)},
}

func (writer Writer) composedConfig(components []Component, options Options) (map[string]json.RawMessage, string, error) {
	body, err := writer.sharedConfigBody(components)
	if err != nil {
		return nil, "", err
	}
	for _, component := range components {
		if err := writer.composeComponent(body, component); err != nil {
			return nil, "", err
		}
	}
	if !hasKind(components, KindText) {
		setJSONBool(body, "nomodel", true)
	}
	applyOptions(body, options)
	if err := validateComposedVLLMConfig(body, components); err != nil {
		return nil, "", err
	}
	if err := validateComposedRuntimeConfig(body); err != nil {
		return nil, "", err
	}
	return body, rawJSONString(body["sdmodel"]), nil
}

func (writer Writer) sharedConfigBody(components []Component) (map[string]json.RawMessage, error) {
	body := map[string]json.RawMessage{}
	for _, component := range components {
		if componentSource(component) != SourceConfig {
			continue
		}
		source, err := writer.configBody(component)
		if err != nil {
			return nil, err
		}
		copySharedKeys(body, source)
		break
	}
	return body, nil
}

func (writer Writer) composeComponent(body map[string]json.RawMessage, component Component) error {
	composer, ok := componentComposers[component.Kind]
	if !ok {
		return fmt.Errorf("component kind %q is invalid", component.Kind)
	}
	if componentSource(component) == SourceFile {
		filePath, err := writer.validateRawFile(component.FilePath)
		if err != nil {
			return err
		}
		setJSONString(body, composer.fileOptionKey(component), filePath)
	} else {
		sourceBody, err := writer.configBody(component)
		if err != nil {
			return err
		}
		composer.copySettings(body, sourceBody)
	}
	if composer.enablesText {
		setJSONBool(body, "nomodel", false)
	}
	return nil
}

func NormalizedComponents(components []Component) ([]Component, error) {
	if len(components) == 0 {
		return nil, fmt.Errorf("at least one component is required")
	}
	result := make([]Component, 0, len(components))
	seen := map[string]struct{}{}
	for _, component := range components {
		component = trimmedComponent(component)
		if _, ok := seen[component.Kind]; ok {
			return nil, fmt.Errorf("duplicate %s component", component.Kind)
		}
		if err := validateNormalizedComponent(component); err != nil {
			return nil, err
		}
		seen[component.Kind] = struct{}{}
		result = append(result, component)
	}
	return result, nil
}

func trimmedComponent(component Component) Component {
	component.Kind = strings.TrimSpace(component.Kind)
	component.Source = componentSource(component)
	component.NodeID = strings.TrimSpace(component.NodeID)
	component.NodeURL = strings.TrimSpace(component.NodeURL)
	component.ModelID = strings.TrimSpace(component.ModelID)
	component.ImageID = strings.TrimSpace(component.ImageID)
	component.FilePath = strings.TrimSpace(component.FilePath)
	component.OptionKey = strings.TrimSpace(component.OptionKey)
	return component
}

func validateNormalizedComponent(component Component) error {
	if _, known := componentComposers[component.Kind]; !known {
		return fmt.Errorf("component kind %q is invalid", component.Kind)
	}
	switch component.Source {
	case SourceConfig:
		if component.ModelID == "" && component.ImageID == "" {
			return fmt.Errorf("%s model id is required", component.Kind)
		}
		return nil
	case SourceFile:
		return validateFileComponent(component)
	default:
		return fmt.Errorf("%s source %q is invalid", component.Kind, component.Source)
	}
}

func validateFileComponent(component Component) error {
	if component.FilePath == "" {
		return fmt.Errorf("%s file path is required", component.Kind)
	}
	if (component.Kind == KindVoice || component.Kind == KindMusic) && !validRawFileOptionKey(component.Kind, component.OptionKey) {
		return fmt.Errorf("%s option key %q is invalid", component.Kind, component.OptionKey)
	}
	return nil
}
