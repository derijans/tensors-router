package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

func decodeServers(content []byte, backend string) ([]server, bool, string, error) {
	value, err := decodeUniqueJSON(content)
	if err != nil {
		return nil, false, "", err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, false, "", fmt.Errorf("configuration must be a JSON object")
	}
	if _, legacy := root["mcpfile"]; legacy {
		return nil, false, "", fmt.Errorf("mcpfile is not supported; use mcp_servers")
	}
	if configuredBackend, exists := root["backend_mode"].(string); exists && configuredBackend != "" {
		backend = configuredBackend
	}
	if backend != BackendKobold && backend != BackendLlama {
		return nil, false, "", fmt.Errorf("backend mode is invalid")
	}
	enabled, err := mcpEnabled(root)
	if err != nil {
		return nil, false, "", err
	}
	rawServers, exists := root["mcp_servers"]
	if !exists {
		return nil, enabled, backend, nil
	}
	servers, err := decodeServerEntries(rawServers, backend)
	if err != nil {
		return nil, false, "", err
	}
	return servers, enabled, backend, nil
}

func mcpEnabled(root map[string]any) (bool, error) {
	rawEnabled, hasEnabled := root["mcp_enabled"]
	if !hasEnabled {
		return false, nil
	}
	enabled, ok := rawEnabled.(bool)
	if !ok {
		return false, fmt.Errorf("mcp_enabled must be a boolean")
	}
	return enabled, nil
}

func decodeServerEntries(rawServers any, backend string) ([]server, error) {
	array, ok := rawServers.([]any)
	if !ok {
		return nil, fmt.Errorf("mcp_servers must be an array")
	}
	servers := make([]server, 0, len(array))
	seen := make(map[string]struct{}, len(array))
	for _, item := range array {
		decoded, err := decodeServerEntry(item, backend, seen)
		if err != nil {
			return nil, err
		}
		seen[decoded.Name] = struct{}{}
		servers = append(servers, decoded)
	}
	return servers, nil
}

func decodeServerEntry(item any, backend string, seen map[string]struct{}) (server, error) {
	entry, ok := item.(map[string]any)
	if !ok {
		return server{}, fmt.Errorf("mcp_servers entries must be objects")
	}
	name, ok := entry["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return server{}, fmt.Errorf("mcp server name is required")
	}
	if _, exists := seen[name]; exists {
		return server{}, fmt.Errorf("duplicate mcp server name %q", name)
	}
	definition, ok := entry["definition"].(map[string]any)
	if !ok {
		return server{}, fmt.Errorf("mcp server %q definition must be an object", name)
	}
	if err := validateDefinition(definition, backend); err != nil {
		return server{}, fmt.Errorf("mcp server %q: %w", name, err)
	}
	encoded, err := json.Marshal(definition)
	if err != nil {
		return server{}, err
	}
	return server{Name: name, Definition: encoded}, nil
}

func validateDefinition(definition map[string]any, backend string) error {
	command, hasCommand := definition["command"]
	addressValue, hasURL := definition["url"]
	if hasCommand == hasURL {
		return fmt.Errorf("definition requires exactly one of command or url")
	}
	if hasCommand {
		return validateCommandDefinition(definition, command)
	}
	if backend != BackendKobold {
		return fmt.Errorf("HTTP(S) transport is not supported by this backend")
	}
	return validateURLDefinition(definition, addressValue)
}

func validateCommandDefinition(definition map[string]any, command any) error {
	if text, ok := command.(string); !ok || strings.TrimSpace(text) == "" {
		return fmt.Errorf("command must be a non-empty string")
	}
	if args, exists := definition["args"]; exists && !stringArray(args) {
		return fmt.Errorf("args must be an array of strings")
	}
	if env, exists := definition["env"]; exists && !stringMap(env) {
		return fmt.Errorf("env must be an object containing string values")
	}
	return nil
}

func validateURLDefinition(definition map[string]any, addressValue any) error {
	address, ok := addressValue.(string)
	if !ok || !httpURL(address) {
		return fmt.Errorf("url must be an HTTP(S) URL")
	}
	if headers, exists := definition["headers"]; exists && !stringMap(headers) {
		return fmt.Errorf("headers must be an object containing string values")
	}
	return nil
}

func httpURL(address string) bool {
	parsed, err := url.Parse(address)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.Fragment != "" {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")
}

func stringArray(value any) bool {
	values, ok := value.([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

func stringMap(value any) bool {
	values, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

func decodeUniqueJSON(content []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	value, err := decodeUniqueValue(decoder)
	if err != nil {
		return nil, err
	}
	if token, err := decoder.Token(); err != io.EOF || token != nil {
		return nil, fmt.Errorf("configuration contains trailing JSON")
	}
	return value, nil
}

func decodeUniqueValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}
	switch delimiter {
	case '{':
		return decodeUniqueObject(decoder)
	case '[':
		return decodeUniqueArray(decoder)
	default:
		return nil, fmt.Errorf("JSON delimiter is invalid")
	}
}

func decodeUniqueObject(decoder *json.Decoder) (map[string]any, error) {
	result := map[string]any{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok {
			return nil, fmt.Errorf("JSON object key is invalid")
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("duplicate JSON key %q", name)
		}
		if result[name], err = decodeUniqueValue(decoder); err != nil {
			return nil, err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return result, nil
}

func decodeUniqueArray(decoder *json.Decoder) ([]any, error) {
	result := []any{}
	for decoder.More() {
		value, err := decodeUniqueValue(decoder)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return result, nil
}
