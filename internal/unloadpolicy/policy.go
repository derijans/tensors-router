package unloadpolicy

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	Key        = "router_unload_policy"
	None       = "none"
	Text       = "text"
	Image      = "image"
	Embeddings = "embeddings"
	Voice      = "voice"
	Music      = "music"
	All        = "all"
)

func Values() []string {
	return []string{None, Text, Image, Embeddings, Voice, Music, All}
}

func Targets() []string {
	return []string{Text, Image, Embeddings, Voice, Music, All}
}

func Resolve(value string) (string, error) {
	value = Normalize(value)
	if value == "" {
		return None, nil
	}
	if Valid(value) {
		return value, nil
	}
	return "", fmt.Errorf("%s must be one of: %s", Key, strings.Join(Values(), ", "))
}

func ResolveTarget(value string) (string, error) {
	value = Normalize(value)
	if value == "" {
		return All, nil
	}
	if ValidTarget(value) {
		return value, nil
	}
	return "", fmt.Errorf("unload target must be one of: %s", strings.Join(Targets(), ", "))
}

func ResolveRaw(options map[string]json.RawMessage) (string, bool, error) {
	raw, ok := options[Key]
	if !ok || len(raw) == 0 || strings.EqualFold(strings.TrimSpace(string(raw)), "null") {
		return None, false, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", true, err
	}
	resolved, err := Resolve(value)
	return resolved, true, err
}

func Valid(value string) bool {
	switch Normalize(value) {
	case None, Text, Image, Embeddings, Voice, Music, All:
		return true
	default:
		return false
	}
}

func ValidTarget(value string) bool {
	switch Normalize(value) {
	case Text, Image, Embeddings, Voice, Music, All:
		return true
	default:
		return false
	}
}

func Normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
