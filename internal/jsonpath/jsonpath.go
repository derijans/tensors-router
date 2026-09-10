package jsonpath

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

func DecodeObject(body []byte) (map[string]any, bool) {
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, false
	}
	root, ok := payload.(map[string]any)
	return root, ok
}

func FirstNumber(root map[string]any, paths ...[]string) float64 {
	for _, path := range paths {
		if value, ok := Number(root, path); ok {
			return value
		}
	}
	return 0
}

func FirstString(root map[string]any, paths ...[]string) string {
	for _, path := range paths {
		value, ok := Resolve(root, path)
		if !ok {
			continue
		}
		if text, ok := value.(string); ok {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func Number(root map[string]any, path []string) (float64, bool) {
	value, ok := Resolve(root, path)
	if !ok {
		return 0, false
	}
	return NumberValue(value)
}

func Array(root map[string]any, path []string) ([]any, bool) {
	value, ok := Resolve(root, path)
	if !ok {
		return nil, false
	}
	values, ok := value.([]any)
	return values, ok
}

func Resolve(root map[string]any, path []string) (any, bool) {
	var current any = root
	for _, segment := range path {
		next, ok := resolveSegment(current, segment)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func resolveSegment(current any, segment string) (any, bool) {
	switch container := current.(type) {
	case map[string]any:
		value, ok := container[segment]
		return value, ok
	case []any:
		index, err := strconv.Atoi(segment)
		if err != nil || index < 0 || index >= len(container) {
			return nil, false
		}
		return container[index], true
	default:
		return nil, false
	}
}

func NumberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}
