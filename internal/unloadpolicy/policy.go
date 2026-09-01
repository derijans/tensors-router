package unloadpolicy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"tensors-router/internal/backendmode"
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

	FamilyPrefix = "family:"
	ConfigPrefix = "config:"
)

// Values is the legacy single-valued vocabulary; each stays valid inside a Selection.
func Values() []string {
	return []string{None, Text, Image, Embeddings, Voice, Music, All}
}

// Triggers is the enumerable vocabulary for config authors. config:<model-id>
// triggers are open-ended and not listed.
func Triggers() []string {
	return append(Values(), FamilyTriggerValues()...)
}

func FamilyTriggerValues() []string {
	return []string{
		FamilyPrefix + backendmode.Kobold,
		FamilyPrefix + backendmode.LlamaSDCPP,
		FamilyPrefix + backendmode.VLLM,
	}
}

func Targets() []string {
	return []string{Text, Image, Embeddings, Voice, Music, All}
}

// Selection unmarshals from a bare JSON string (the legacy .kcpps shape) or an
// array, and always marshals back to an array.
type Selection []string

func (selection Selection) MarshalJSON() ([]byte, error) {
	if len(selection) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(selection))
}

func (selection *Selection) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*selection = nil
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var values []string
		if err := json.Unmarshal(data, &values); err != nil {
			return err
		}
		*selection = Selection(values)
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	if strings.TrimSpace(single) == "" {
		*selection = nil
		return nil
	}
	*selection = Selection{single}
	return nil
}

func (selection Selection) Contains(trigger string) bool {
	trigger = normalizeTrigger(trigger)
	for _, value := range selection {
		if normalizeTrigger(value) == trigger {
			return true
		}
	}
	return false
}

func (selection Selection) IsNone() bool {
	resolved, err := ResolveSelection(selection)
	return err == nil && len(resolved) == 1 && resolved[0] == None
}

// ResolveSelection normalizes, validates, dedupes and sorts a trigger set. Empty
// resolves to {none}; none and all cannot be combined with any other trigger.
func ResolveSelection(selection Selection) (Selection, error) {
	seen := map[string]struct{}{}
	resolved := make([]string, 0, len(selection))
	for _, raw := range selection {
		value := normalizeTrigger(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		resolved = append(resolved, value)
	}
	if len(resolved) == 0 {
		return Selection{None}, nil
	}
	sort.Strings(resolved)
	for _, value := range resolved {
		if (value == None || value == All) && len(resolved) > 1 {
			return nil, fmt.Errorf("%s trigger %q cannot be combined with other triggers", Key, value)
		}
		if !ValidTrigger(value) {
			return nil, fmt.Errorf("%s trigger %q is not one of: %s, config:<model-id>", Key, value, strings.Join(Triggers(), ", "))
		}
	}
	return Selection(resolved), nil
}

func ResolveSelectionRaw(options map[string]json.RawMessage) (Selection, bool, error) {
	raw, ok := options[Key]
	if !ok || len(raw) == 0 || strings.EqualFold(strings.TrimSpace(string(raw)), "null") {
		return Selection{None}, false, nil
	}
	var selection Selection
	if err := selection.UnmarshalJSON(raw); err != nil {
		return nil, true, err
	}
	resolved, err := ResolveSelection(selection)
	return resolved, true, err
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

func ValidTrigger(value string) bool {
	value = normalizeTrigger(value)
	switch value {
	case None, All, Text, Image, Embeddings, Voice, Music:
		return true
	}
	return ValidFamilyTrigger(value) || ValidConfigTrigger(value)
}

func ValidLane(value string) bool {
	switch Normalize(value) {
	case Text, Image, Embeddings, Voice, Music:
		return true
	default:
		return false
	}
}

func ValidFamilyTrigger(value string) bool {
	suffix, ok := strings.CutPrefix(normalizeTrigger(value), FamilyPrefix)
	return ok && backendmode.Valid(suffix)
}

func ValidConfigTrigger(value string) bool {
	suffix, ok := strings.CutPrefix(normalizeTrigger(value), ConfigPrefix)
	return ok && strings.TrimSpace(suffix) != ""
}

func ValidTarget(value string) bool {
	switch Normalize(value) {
	case Text, Image, Embeddings, Voice, Music, All:
		return true
	default:
		return false
	}
}

func FamilyTarget(value string) (string, bool) {
	suffix, ok := strings.CutPrefix(normalizeTrigger(value), FamilyPrefix)
	if !ok || !backendmode.Valid(suffix) {
		return "", false
	}
	return suffix, true
}

// ConfigTarget returns the model id from a config:<model-id> trigger, keeping the
// id's original case.
func ConfigTarget(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if before, after, found := strings.Cut(trimmed, ":"); found && strings.EqualFold(strings.TrimSpace(before), "config") {
		id := strings.TrimSpace(after)
		return id, id != ""
	}
	return "", false
}

func Normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// normalizeTrigger lowercases a trigger but keeps a config:<model-id> suffix as-is,
// since model ids are matched exactly.
func normalizeTrigger(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if before, after, found := strings.Cut(trimmed, ":"); found {
		switch strings.ToLower(strings.TrimSpace(before)) {
		case "family":
			return FamilyPrefix + strings.ToLower(strings.TrimSpace(after))
		case "config":
			return ConfigPrefix + strings.TrimSpace(after)
		}
	}
	return strings.ToLower(trimmed)
}
