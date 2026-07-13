package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

const (
	JinjaKwargsKey                 = "jinja_kwargs"
	RouterJinjaKwargsPrecedenceKey = "router_jinja_kwargs_precedence"
	JinjaKwargsPrecedenceConfig    = "config"
	JinjaKwargsPrecedenceClient    = "client"
	defaultJinjaKwargsPrecedence   = JinjaKwargsPrecedenceConfig
)

type ChatTemplateProfile struct {
	kwargs                  json.RawMessage
	configured              bool
	precedence              string
	valid                   bool
	physicalLoadFingerprint string
}

func ChatTemplateProfileForConfig(content []byte) ChatTemplateProfile {
	values, err := decodeConfigObject(content)
	if err != nil {
		return ChatTemplateProfile{}
	}

	kwargs, configured, err := NormalizeJinjaKwargs(values[JinjaKwargsKey])
	if err != nil {
		return ChatTemplateProfile{}
	}
	precedence, _, err := NormalizeJinjaKwargsPrecedence(values[RouterJinjaKwargsPrecedenceKey])
	if err != nil {
		return ChatTemplateProfile{}
	}
	fingerprint, err := physicalLoadFingerprint(content, kwargs)
	if err != nil {
		return ChatTemplateProfile{}
	}
	return ChatTemplateProfile{
		kwargs:                  cloneRawMessage(kwargs),
		configured:              configured,
		precedence:              precedence,
		valid:                   true,
		physicalLoadFingerprint: fingerprint,
	}
}

func NormalizeJinjaKwargs(value json.RawMessage) (json.RawMessage, bool, error) {
	value = bytes.TrimSpace(value)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return nil, false, nil
	}
	if value[0] == '"' {
		var encoded string
		if err := json.Unmarshal(value, &encoded); err != nil {
			return nil, false, fmt.Errorf("%s must be a JSON-encoded object: %w", JinjaKwargsKey, err)
		}
		value = bytes.TrimSpace([]byte(encoded))
		if len(value) == 0 || bytes.Equal(value, []byte("null")) {
			return nil, false, nil
		}
	}
	object, err := decodeUniqueJSONObject(value)
	if err != nil {
		return nil, false, fmt.Errorf("%s must be an object: %w", JinjaKwargsKey, err)
	}
	normalized, err := json.Marshal(object)
	if err != nil {
		return nil, false, err
	}
	return normalized, true, nil
}

func NormalizeJinjaKwargsPrecedence(value json.RawMessage) (string, bool, error) {
	value = bytes.TrimSpace(value)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return defaultJinjaKwargsPrecedence, false, nil
	}
	var precedence string
	if err := json.Unmarshal(value, &precedence); err != nil {
		return "", false, fmt.Errorf("%s must be a string", RouterJinjaKwargsPrecedenceKey)
	}
	precedence = strings.ToLower(strings.TrimSpace(precedence))
	switch precedence {
	case JinjaKwargsPrecedenceConfig, JinjaKwargsPrecedenceClient:
		return precedence, true, nil
	default:
		return "", false, fmt.Errorf("%s must be %q or %q", RouterJinjaKwargsPrecedenceKey, JinjaKwargsPrecedenceConfig, JinjaKwargsPrecedenceClient)
	}
}

func (profile ChatTemplateProfile) Valid() bool {
	return profile.valid
}

func (profile ChatTemplateProfile) HasConfiguredKwargs() bool {
	if !profile.valid || !profile.configured {
		return false
	}
	object, err := decodeUniqueJSONObject(profile.kwargs)
	return err == nil && len(object) > 0
}

func (profile ChatTemplateProfile) ConfiguredKwargs() json.RawMessage {
	return cloneRawMessage(profile.kwargs)
}

func (profile ChatTemplateProfile) Precedence() string {
	if profile.precedence == "" {
		return defaultJinjaKwargsPrecedence
	}
	return profile.precedence
}

func (profile ChatTemplateProfile) PhysicalLoadFingerprint() string {
	return profile.physicalLoadFingerprint
}

func (profile ChatTemplateProfile) SharesPhysicalRuntimeWith(other ChatTemplateProfile) bool {
	return profile.valid && other.valid && profile.physicalLoadFingerprint != "" && profile.physicalLoadFingerprint == other.physicalLoadFingerprint
}

func (profile ChatTemplateProfile) clone() ChatTemplateProfile {
	cloned := profile
	cloned.kwargs = cloneRawMessage(profile.kwargs)
	return cloned
}

func physicalLoadFingerprint(content []byte, normalizedKwargs json.RawMessage) (string, error) {
	values, err := decodeConfigObject(content)
	if err != nil {
		return "", err
	}
	keys := []string{}
	if len(normalizedKwargs) > 0 {
		kwargs, err := decodeUniqueJSONObject(normalizedKwargs)
		if err != nil {
			return "", err
		}
		for key := range kwargs {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	delete(values, RouterJinjaKwargsPrecedenceKey)
	keySet, err := json.Marshal(keys)
	if err != nil {
		return "", err
	}
	values[JinjaKwargsKey] = keySet
	canonical, err := canonicalJSONObject(values)
	if err != nil {
		return "", err
	}
	return hashBytes(canonical), nil
}

func decodeConfigObject(content []byte) (map[string]json.RawMessage, error) {
	return decodeUniqueJSONObject(content)
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
		values[key] = cloneRawMessage(value)
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
			var item json.RawMessage
			if err := decoder.Decode(&item); err != nil {
				return err
			}
			if err := validateUniqueJSON(item); err != nil {
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

func canonicalJSONObject(values map[string]json.RawMessage) ([]byte, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var output bytes.Buffer
	output.WriteByte('{')
	for index, key := range keys {
		if index > 0 {
			output.WriteByte(',')
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		output.Write(encodedKey)
		output.WriteByte(':')
		canonicalValue, err := canonicalJSONValue(values[key])
		if err != nil {
			return nil, err
		}
		output.Write(canonicalValue)
	}
	output.WriteByte('}')
	return output.Bytes(), nil
}

func canonicalJSONValue(content json.RawMessage) ([]byte, error) {
	content = bytes.TrimSpace(content)
	if len(content) == 0 || !json.Valid(content) {
		return nil, fmt.Errorf("value is not valid JSON")
	}
	switch content[0] {
	case '{':
		values, err := decodeUniqueJSONObject(content)
		if err != nil {
			return nil, err
		}
		return canonicalJSONObject(values)
	case '[':
		decoder := json.NewDecoder(bytes.NewReader(content))
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		var output bytes.Buffer
		output.WriteByte('[')
		index := 0
		for decoder.More() {
			var item json.RawMessage
			if err := decoder.Decode(&item); err != nil {
				return nil, err
			}
			if index > 0 {
				output.WriteByte(',')
			}
			canonical, err := canonicalJSONValue(item)
			if err != nil {
				return nil, err
			}
			output.Write(canonical)
			index++
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		output.WriteByte(']')
		return output.Bytes(), nil
	default:
		return append([]byte(nil), content...), nil
	}
}

func cloneRawMessage(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}
