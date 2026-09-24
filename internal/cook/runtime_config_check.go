package cook

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"tensors-router/internal/catalog"
)

func validateComposedRuntimeConfig(body map[string]json.RawMessage) error {
	issues := runtimeConfigDecodeIssues(body)
	issues = append(issues, runtimeConfigShapeIssues(body)...)
	if len(issues) == 0 {
		return nil
	}
	return ValidationError{Issues: issues}
}

func runtimeConfigDecodeIssues(body map[string]json.RawMessage) []ValidationIssue {
	content, err := json.Marshal(body)
	if err != nil {
		return nil
	}
	metadata, err := catalog.DecodeRuntimeConfig(content)
	if err != nil {
		field := ""
		var typeError *json.UnmarshalTypeError
		if errors.As(err, &typeError) {
			field = typeError.Field
		}
		return []ValidationIssue{{
			Severity: "error",
			Code:     "runtime_config_undecodable",
			Field:    field,
			Message:  fmt.Sprintf("configuration cannot be loaded by the router: %v", err),
		}}
	}
	return runtimeConfigValueIssues(metadata)
}

func runtimeConfigValueIssues(metadata catalog.RuntimeConfig) []ValidationIssue {
	if _, err := metadata.LlamaLoadMode(); err != nil {
		return []ValidationIssue{{
			Severity: "error",
			Code:     "option_value_choice",
			Field:    "load_mode",
			Message:  err.Error(),
		}}
	}
	return nil
}

func runtimeConfigShapeIssues(body map[string]json.RawMessage) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	for key, value := range body {
		definition, ok := OptionDefinitionForKey(strings.TrimPrefix(key, "llama_"))
		if !ok {
			continue
		}
		if message, mismatched := runtimeConfigShapeMismatch(definition.ValueType, value); mismatched {
			issues = append(issues, ValidationIssue{
				Severity: "error",
				Code:     "option_value_shape",
				Field:    key,
				Message:  fmt.Sprintf("%s: %s", key, message),
			})
		}
	}
	return issues
}

func runtimeConfigShapeMismatch(valueType string, value json.RawMessage) (string, bool) {
	token := strings.TrimSpace(string(value))
	if token == "" || token == "null" {
		return "", false
	}
	switch valueType {
	case ValueNumber:
		if !isJSONNumberToken(token) {
			return "expected a number, got " + token, true
		}
	case ValueBool:
		if token != "true" && token != "false" {
			return "expected true or false, got " + token, true
		}
	case ValueStringList:
		if !strings.HasPrefix(token, "[") {
			return "expected a list, got " + token, true
		}
	}
	return "", false
}

func isJSONNumberToken(token string) bool {
	if token == "" {
		return false
	}
	index := 0
	if token[0] == '-' {
		index = 1
	}
	if index >= len(token) {
		return false
	}
	for ; index < len(token); index++ {
		character := token[index]
		if character >= '0' && character <= '9' {
			continue
		}
		if character == '.' || character == 'e' || character == 'E' || character == '+' || character == '-' {
			continue
		}
		return false
	}
	return true
}
