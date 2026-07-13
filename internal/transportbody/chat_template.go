package transportbody

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const maxChatTemplateKwargsBytes = 32 * MiB

func mergeChatTemplateKwargs(configured json.RawMessage, client json.RawMessage, configWins bool) ([]byte, error) {
	configuredValues, err := decodeChatTemplateKwargsObject(configured)
	if err != nil {
		return nil, err
	}
	client = bytes.TrimSpace(client)
	clientValues := map[string]json.RawMessage{}
	if len(client) > 0 && !bytes.Equal(client, []byte("null")) {
		clientValues, err = decodeChatTemplateKwargsObject(client)
		if err != nil {
			return nil, err
		}
	}
	merged := cloneJSONObject(configuredValues)
	if configWins {
		merged = cloneJSONObject(clientValues)
		copyJSONObject(merged, configuredValues)
	} else {
		copyJSONObject(merged, clientValues)
	}
	result, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidChatTemplateKwargs, err)
	}
	return result, nil
}

func decodeChatTemplateKwargsObject(content []byte) (map[string]json.RawMessage, error) {
	values, err := decodeUniqueJSONObject(content)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidChatTemplateKwargs, err)
	}
	return values, nil
}

func cloneJSONObject(values map[string]json.RawMessage) map[string]json.RawMessage {
	cloned := make(map[string]json.RawMessage, len(values))
	copyJSONObject(cloned, values)
	return cloned
}

func copyJSONObject(destination map[string]json.RawMessage, source map[string]json.RawMessage) {
	for key, value := range source {
		destination[key] = append(json.RawMessage(nil), value...)
	}
}

func readRawJSONValue(reader *bufio.Reader, first byte) ([]byte, error) {
	content := []byte{first}
	appendByte := func(value byte) error {
		if int64(len(content)) >= maxChatTemplateKwargsBytes {
			return ErrChatTemplateKwargsTooLarge
		}
		content = append(content, value)
		return nil
	}
	if first == '{' || first == '[' {
		depth := 1
		inString := false
		escaped := false
		for depth > 0 {
			value, err := reader.ReadByte()
			if err != nil {
				return nil, rawJSONReadError(err)
			}
			if err := appendByte(value); err != nil {
				return nil, err
			}
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if value == '\\' {
					escaped = true
					continue
				}
				if value == '"' {
					inString = false
				}
				continue
			}
			switch value {
			case '"':
				inString = true
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
		if !json.Valid(content) {
			return nil, ErrInvalidJSON
		}
		return content, nil
	}
	if first == '"' {
		escaped := false
		for {
			value, err := reader.ReadByte()
			if err != nil {
				return nil, rawJSONReadError(err)
			}
			if err := appendByte(value); err != nil {
				return nil, err
			}
			if escaped {
				escaped = false
				continue
			}
			if value == '\\' {
				escaped = true
				continue
			}
			if value == '"' {
				break
			}
		}
		if !json.Valid(content) {
			return nil, ErrInvalidJSON
		}
		return content, nil
	}
	for {
		value, err := reader.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if isJSONDelimiter(value) {
			if err := reader.UnreadByte(); err != nil {
				return nil, err
			}
			break
		}
		if err := appendByte(value); err != nil {
			return nil, err
		}
	}
	if !json.Valid(content) {
		return nil, ErrInvalidJSON
	}
	return content, nil
}

func rawJSONReadError(err error) error {
	if errors.Is(err, ErrRequestTooLarge) {
		return err
	}
	return ErrInvalidJSON
}

func decodeUniqueJSONObject(content []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	start, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok := start.(json.Delim)
	if !ok || delimiter != '{' {
		return nil, fmt.Errorf("value is not an object")
	}
	values := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("object key is invalid")
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if err := validateUniqueJSON(value); err != nil {
			return nil, err
		}
		values[key] = append(json.RawMessage(nil), value...)
	}
	end, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok = end.(json.Delim)
	if !ok || delimiter != '}' {
		return nil, fmt.Errorf("object is not closed")
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing value")
		}
		return nil, err
	}
	return values, nil
}

func validateUniqueJSON(content []byte) error {
	content = bytes.TrimSpace(content)
	if len(content) == 0 || !json.Valid(content) {
		return fmt.Errorf("value is not valid JSON")
	}
	switch content[0] {
	case '{':
		_, err := decodeUniqueJSONObject(content)
		return err
	case '[':
		decoder := json.NewDecoder(bytes.NewReader(content))
		start, err := decoder.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := start.(json.Delim); !ok || delimiter != '[' {
			return fmt.Errorf("value is not an array")
		}
		for decoder.More() {
			var value json.RawMessage
			if err := decoder.Decode(&value); err != nil {
				return err
			}
			if err := validateUniqueJSON(value); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := end.(json.Delim); !ok || delimiter != ']' {
			return fmt.Errorf("array is not closed")
		}
		if _, err := decoder.Token(); err != io.EOF {
			if err == nil {
				return fmt.Errorf("unexpected trailing value")
			}
			return err
		}
	}
	return nil
}
