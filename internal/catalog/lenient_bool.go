package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
)

type LenientBool bool

func (value *LenientBool) UnmarshalJSON(raw []byte) error {
	var enabled bool
	if err := json.Unmarshal(raw, &enabled); err == nil {
		*value = LenientBool(enabled)
		return nil
	}
	var legacy string
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return fmt.Errorf("expected true or false, got %s", raw)
	}
	switch strings.ToLower(strings.TrimSpace(legacy)) {
	case "true", "on":
		*value = true
	case "false", "off":
		*value = false
	default:
		return fmt.Errorf("expected true or false, got %q", legacy)
	}
	return nil
}

func (value *LenientBool) Bool() *bool {
	if value == nil {
		return nil
	}
	enabled := bool(*value)
	return &enabled
}
