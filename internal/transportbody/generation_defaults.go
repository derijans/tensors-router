package transportbody

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const generationOptionsField = "options"

type JSONField struct {
	Key   string
	Value json.RawMessage
}

type GenerationDefaultsRewrite struct {
	Defaults          []JSONField
	Overwrite         bool
	IntoOllamaOptions bool
}

type generationDefaults struct {
	ordered     []JSONField
	byKey       map[string]json.RawMessage
	overwrite   bool
	intoOptions bool
}

func newGenerationDefaults(rewrite *GenerationDefaultsRewrite) (*generationDefaults, error) {
	if rewrite == nil || len(rewrite.Defaults) == 0 {
		return nil, nil
	}
	defaults := &generationDefaults{byKey: make(map[string]json.RawMessage, len(rewrite.Defaults)), overwrite: rewrite.Overwrite, intoOptions: rewrite.IntoOllamaOptions}
	for _, field := range rewrite.Defaults {
		value := bytes.TrimSpace(field.Value)
		if field.Key == "" || validateUniqueJSON(value) != nil {
			return nil, fmt.Errorf("%w: %q", ErrInvalidGenerationDefault, field.Key)
		}
		if _, duplicate := defaults.byKey[field.Key]; duplicate {
			return nil, fmt.Errorf("%w: %q", ErrInvalidGenerationDefault, field.Key)
		}
		value = append(json.RawMessage(nil), value...)
		defaults.byKey[field.Key] = value
		defaults.ordered = append(defaults.ordered, JSONField{Key: field.Key, Value: value})
	}
	return defaults, nil
}

func (defaults *generationDefaults) targetsRoot() bool {
	return defaults != nil && !defaults.intoOptions
}

func (defaults *generationDefaults) targetsOptions() bool {
	return defaults != nil && defaults.intoOptions
}

func (defaults *generationDefaults) has(key string) bool {
	_, found := defaults.byKey[key]
	return found
}

func (processor *jsonProcessor) noteGenerationDefaultKey(container *jsonContainer) error {
	defaults := processor.generationDefaults
	if container.generationDefaultsTarget && defaults.has(container.key) {
		if container.seenGenerationDefaults[container.key] {
			return fmt.Errorf("%w: %q", ErrDuplicateGenerationDefault, container.key)
		}
		container.seenGenerationDefaults[container.key] = true
	}
	if container.root && defaults.targetsOptions() && container.key == generationOptionsField {
		if processor.generationOptionsSeen {
			return fmt.Errorf("%w: %q", ErrDuplicateGenerationDefault, generationOptionsField)
		}
		processor.generationOptionsSeen = true
	}
	return nil
}

func (processor *jsonProcessor) overwritesGenerationDefault(parent jsonContainer) bool {
	defaults := processor.generationDefaults
	return parent.generationDefaultsTarget && defaults.overwrite && defaults.has(parent.key)
}

func (processor *jsonProcessor) writeOverwrittenGenerationDefault(value byte, key string) error {
	if _, err := readRawJSONValue(processor.reader, value); err != nil {
		return err
	}
	_, err := processor.writer.Write(processor.generationDefaults.byKey[key])
	return err
}

func (processor *jsonProcessor) holdsGenerationOptions(parent jsonContainer) bool {
	return parent.root && processor.generationDefaults.targetsOptions() && parent.key == generationOptionsField
}

func (processor *jsonProcessor) consumeGenerationOptions(value byte) error {
	if value == '{' {
		return processor.openContainer(value, newGenerationDefaultsTarget(jsonContainer{kind: '{', state: objectFirstKeyState}))
	}
	raw, err := readRawJSONValue(processor.reader, value)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, []byte("null")) {
		return ErrInvalidGenerationOptions
	}
	return processor.writeGenerationDefaultsObject()
}

func newGenerationDefaultsTarget(container jsonContainer) jsonContainer {
	container.generationDefaultsTarget = true
	container.seenGenerationDefaults = map[string]bool{}
	return container
}

func (processor *jsonProcessor) writeMissingGenerationDefaults(container *jsonContainer) error {
	for _, field := range processor.generationDefaults.ordered {
		if container.seenGenerationDefaults[field.Key] {
			continue
		}
		if err := processor.writeRawField(container, field.Key, field.Value); err != nil {
			return err
		}
	}
	return nil
}

func (processor *jsonProcessor) writeGenerationOptionsWhenAbsent(root *jsonContainer) error {
	if !processor.generationDefaults.targetsOptions() || processor.generationOptionsSeen {
		return nil
	}
	if err := processor.writeFieldKey(root, generationOptionsField); err != nil {
		return err
	}
	return processor.writeGenerationDefaultsObject()
}

func (processor *jsonProcessor) writeGenerationDefaultsObject() error {
	options := jsonContainer{}
	if err := processor.writer.WriteByte('{'); err != nil {
		return err
	}
	for _, field := range processor.generationDefaults.ordered {
		if err := processor.writeRawField(&options, field.Key, field.Value); err != nil {
			return err
		}
	}
	return processor.writer.WriteByte('}')
}

func (processor *jsonProcessor) writeRawField(container *jsonContainer, key string, value json.RawMessage) error {
	if err := processor.writeFieldKey(container, key); err != nil {
		return err
	}
	_, err := processor.writer.Write(value)
	return err
}

func (processor *jsonProcessor) writeFieldKey(container *jsonContainer, key string) error {
	if container.fieldCount > 0 {
		if err := processor.writer.WriteByte(','); err != nil {
			return err
		}
	}
	encodedKey, err := json.Marshal(key)
	if err != nil {
		return err
	}
	if _, err := processor.writer.Write(encodedKey); err != nil {
		return err
	}
	container.fieldCount++
	return processor.writer.WriteByte(':')
}
