package transportbody

import (
	"encoding/json"
	"strings"
)

const chatTemplateKwargsField = "chat_template_kwargs"

func (processor *jsonProcessor) consume(value byte) error {
	index := len(processor.stack) - 1
	if processor.stack[index].kind == '{' {
		return processor.consumeInObject(value, index)
	}
	return processor.consumeInArray(value, index)
}

func (processor *jsonProcessor) consumeInObject(value byte, index int) error {
	container := &processor.stack[index]
	switch container.state {
	case objectFirstKeyState, objectKeyState:
		return processor.consumeObjectKey(value, index)
	case objectColonState:
		if value != ':' {
			return ErrInvalidJSON
		}
		container.state = objectValueState
		return processor.writer.WriteByte(value)
	case objectValueState:
		return processor.consumeValue(value, index)
	case objectCommaState:
		return processor.consumeObjectSeparator(value, container)
	default:
		return ErrInvalidJSON
	}
}

func (processor *jsonProcessor) consumeObjectKey(value byte, index int) error {
	if value == '}' && processor.stack[index].state == objectFirstKeyState {
		return processor.closeContainer(value)
	}
	if value != '"' {
		return ErrInvalidJSON
	}
	if err := processor.writer.WriteByte(value); err != nil {
		return err
	}
	raw, overflow, err := processor.copyString(selectorValueLimit, processor.rewrite.EscapeHTML)
	if err != nil {
		return err
	}
	container := &processor.stack[index]
	container.key = ""
	if !overflow {
		if container.key, err = decodeJSONString(raw); err != nil {
			return ErrInvalidJSON
		}
	}
	if container.root {
		if err := processor.countRootField(container); err != nil {
			return err
		}
	}
	container.state = objectColonState
	return nil
}

func (processor *jsonProcessor) countRootField(container *jsonContainer) error {
	container.fieldCount++
	if processor.rewrite.ChatTemplateKwargs == nil || container.key != chatTemplateKwargsField {
		return nil
	}
	if container.chatTemplateKwargsSeen {
		return ErrDuplicateChatTemplateKwargs
	}
	container.chatTemplateKwargsSeen = true
	return nil
}

func (processor *jsonProcessor) consumeObjectSeparator(value byte, container *jsonContainer) error {
	switch value {
	case ',':
		container.key = ""
		container.state = objectKeyState
		return processor.writer.WriteByte(value)
	case '}':
		return processor.closeContainer(value)
	default:
		return ErrInvalidJSON
	}
}

func (processor *jsonProcessor) consumeInArray(value byte, index int) error {
	container := &processor.stack[index]
	switch container.state {
	case arrayFirstValueState, arrayValueState:
		if value == ']' && container.state == arrayFirstValueState {
			return processor.closeContainer(value)
		}
		return processor.consumeValue(value, index)
	case arrayCommaState:
		switch value {
		case ',':
			container.state = arrayValueState
			return processor.writer.WriteByte(value)
		case ']':
			return processor.closeContainer(value)
		default:
			return ErrInvalidJSON
		}
	default:
		return ErrInvalidJSON
	}
}

func (processor *jsonProcessor) consumeValue(value byte, parentIndex int) error {
	parent := &processor.stack[parentIndex]
	path := processor.valuePath(*parent)
	if parent.kind == '{' {
		parent.state = objectCommaState
	} else {
		parent.state = arrayCommaState
	}
	if replacementPath(path) && value != '"' {
		return ErrInvalidJSON
	}
	if processor.shouldMergeChatTemplateKwargs(*parent, path) {
		return processor.writeMergedChatTemplateKwargs(value)
	}
	switch value {
	case '"':
		return processor.consumeStringValue(path)
	case '{':
		override := parent.root && parent.key == "override_settings"
		return processor.openContainer(value, jsonContainer{kind: '{', state: objectFirstKeyState, override: override})
	case '[':
		return processor.openContainer(value, jsonContainer{kind: '[', state: arrayFirstValueState})
	default:
		token, err := processor.copyPrimitive(value)
		if err != nil {
			return err
		}
		processor.assignPrimitive(path, token)
		return nil
	}
}

func (processor *jsonProcessor) writeMergedChatTemplateKwargs(value byte) error {
	client, err := readRawJSONValue(processor.reader, value)
	if err != nil {
		return err
	}
	merged, err := mergeChatTemplateKwargs(processor.rewrite.ChatTemplateKwargs.Configured, client, processor.rewrite.ChatTemplateKwargs.ConfigWins)
	if err != nil {
		return err
	}
	_, err = processor.writer.Write(merged)
	return err
}

func (processor *jsonProcessor) consumeStringValue(path string) error {
	if replacementPath(path) {
		return processor.consumeSelectorString(path)
	}
	if err := processor.writer.WriteByte('"'); err != nil {
		return err
	}
	_, _, err := processor.copyString(0, processor.rewrite.EscapeHTML)
	return err
}

func (processor *jsonProcessor) consumeSelectorString(path string) error {
	raw, err := processor.readTargetString()
	if err != nil {
		return err
	}
	decoded, err := decodeJSONString(raw)
	if err != nil {
		return ErrInvalidJSON
	}
	processor.assignSelector(path, decoded)
	replacement, replace := processor.rewrite.Replacements[path]
	if replace && (replacement.From == "" || strings.TrimSpace(decoded) == strings.TrimSpace(replacement.From)) {
		encoded, err := json.Marshal(replacement.To)
		if err != nil {
			return err
		}
		_, err = processor.writer.Write(encoded)
		return err
	}
	return processor.writeQuotedRawString(raw)
}

func (processor *jsonProcessor) writeQuotedRawString(raw []byte) error {
	if err := processor.writer.WriteByte('"'); err != nil {
		return err
	}
	for _, value := range raw {
		if err := processor.writeStringByte(value, processor.rewrite.EscapeHTML); err != nil {
			return err
		}
	}
	return processor.writer.WriteByte('"')
}

func (processor *jsonProcessor) openContainer(value byte, container jsonContainer) error {
	if len(processor.stack) >= maxJSONNesting {
		return ErrInvalidJSON
	}
	if err := processor.writer.WriteByte(value); err != nil {
		return err
	}
	processor.stack = append(processor.stack, container)
	return nil
}

func (processor *jsonProcessor) closeContainer(value byte) error {
	container := processor.stack[len(processor.stack)-1]
	if container.root {
		if err := processor.writeRootInsertions(&container); err != nil {
			return err
		}
	}
	if err := processor.writer.WriteByte(value); err != nil {
		return err
	}
	processor.stack = processor.stack[:len(processor.stack)-1]
	if len(processor.stack) == 0 {
		processor.complete = true
	}
	return nil
}

func (processor *jsonProcessor) writeRootInsertions(container *jsonContainer) error {
	if !processor.fields.ModelSet {
		if model, ok := processor.rewrite.Insertions[PathModel]; ok {
			if err := processor.writeRootStringField(container, PathModel, model); err != nil {
				return err
			}
		}
	}
	if processor.rewrite.ChatTemplateKwargs == nil || container.chatTemplateKwargsSeen {
		return nil
	}
	return processor.writeConfiguredChatTemplateKwargs(container)
}

func (processor *jsonProcessor) writeConfiguredChatTemplateKwargs(container *jsonContainer) error {
	if container.fieldCount > 0 {
		if err := processor.writer.WriteByte(','); err != nil {
			return err
		}
	}
	encodedKey, err := json.Marshal(chatTemplateKwargsField)
	if err != nil {
		return err
	}
	if _, err := processor.writer.Write(encodedKey); err != nil {
		return err
	}
	if err := processor.writer.WriteByte(':'); err != nil {
		return err
	}
	_, err = processor.writer.Write(processor.rewrite.ChatTemplateKwargs.Configured)
	return err
}
