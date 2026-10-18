package catalog

import (
	"bytes"
	"encoding/json"
	"slices"
	"sort"
	"strconv"

	"tensors-router/internal/backendmode"
)

const (
	GenerationDefaultsKey          = "gendefaults"
	GenerationDefaultsOverwriteKey = "gendefaultsoverwrite"
	llamaRepeatPenaltyName         = "repeat_penalty"
)

var koboldRepeatPenaltyNames = []string{llamaRepeatPenaltyName, "repetition_penalty", "rep_pen"}

var llamaNamesForKoboldSamplers = map[string]string{
	"rep_pen_range": "repeat_last_n",
	"typical":       "typical_p",
}

type GenerationDefault struct {
	Key   string
	Value json.RawMessage
}

type GenerationDefaults struct {
	TopLevel                []GenerationDefault
	OllamaChatReadsTopLevel bool
	OllamaOptions           []GenerationDefault
	Overwrite               bool
}

func (defaults GenerationDefaults) Empty() bool {
	return len(defaults.TopLevel) == 0 && len(defaults.OllamaOptions) == 0
}

func (defaults GenerationDefaults) clone() GenerationDefaults {
	return GenerationDefaults{TopLevel: cloneGenerationDefaults(defaults.TopLevel), OllamaOptions: cloneGenerationDefaults(defaults.OllamaOptions), Overwrite: defaults.Overwrite, OllamaChatReadsTopLevel: defaults.OllamaChatReadsTopLevel}
}

func generationDefaultsForConfig(values map[string]json.RawMessage, backendFamily string) GenerationDefaults {
	configured := parseGenerationDefaultsLikeKoboldCpp(values[GenerationDefaultsKey])
	if len(configured) == 0 {
		return GenerationDefaults{}
	}
	llamaSpelling := llamaGenerationDefaults(configured)
	topLevel := llamaSpelling
	if backendFamily == backendmode.Kobold {
		topLevel = configured
	}
	return GenerationDefaults{
		TopLevel:                sortedGenerationDefaults(topLevel),
		OllamaOptions:           sortedGenerationDefaults(llamaSpelling),
		Overwrite:               generationDefaultsOverwrite(values[GenerationDefaultsOverwriteKey]),
		OllamaChatReadsTopLevel: backendFamily == backendmode.Kobold,
	}
}

func parseGenerationDefaultsLikeKoboldCpp(value json.RawMessage) map[string]json.RawMessage {
	value = bytes.TrimSpace(value)
	var encoded string
	if json.Unmarshal(value, &encoded) == nil {
		value = bytes.TrimSpace([]byte(encoded))
	}
	if len(value) == 0 {
		return nil
	}
	if object, err := decodeUniqueJSONObject(value); err == nil {
		return object
	}
	if !bytes.ContainsRune(value, ':') {
		return nil
	}
	braced := append(append([]byte{'{'}, value...), '}')
	object, err := decodeUniqueJSONObject(braced)
	if err != nil {
		return nil
	}
	return object
}

func generationDefaultsOverwrite(value json.RawMessage) bool {
	var enabled bool
	if json.Unmarshal(value, &enabled) == nil {
		return enabled
	}
	number, err := strconv.ParseFloat(string(bytes.TrimSpace(value)), 64)
	return err == nil && number != 0
}

func llamaGenerationDefaults(configured map[string]json.RawMessage) map[string]json.RawMessage {
	spelled := make(map[string]json.RawMessage, len(configured))
	for key, value := range configured {
		if slices.Contains(koboldRepeatPenaltyNames, key) {
			continue
		}
		if llamaName, renamed := llamaNamesForKoboldSamplers[key]; renamed {
			if _, explicit := configured[llamaName]; explicit {
				continue
			}
			key = llamaName
		}
		spelled[key] = value
	}
	if penalty, found := koboldCppEffectiveRepeatPenalty(configured); found {
		spelled[llamaRepeatPenaltyName] = penalty
	}
	return spelled
}

func koboldCppEffectiveRepeatPenalty(configured map[string]json.RawMessage) (json.RawMessage, bool) {
	var strongest json.RawMessage
	strongestValue := 0.0
	for _, name := range koboldRepeatPenaltyNames {
		value, present := configured[name]
		if !present {
			continue
		}
		number, err := strconv.ParseFloat(string(bytes.TrimSpace(value)), 64)
		if err != nil {
			continue
		}
		if strongest == nil || number > strongestValue {
			strongest, strongestValue = value, number
		}
	}
	return strongest, strongest != nil
}

func sortedGenerationDefaults(values map[string]json.RawMessage) []GenerationDefault {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	defaults := make([]GenerationDefault, 0, len(keys))
	for _, key := range keys {
		defaults = append(defaults, GenerationDefault{Key: key, Value: cloneRawMessage(values[key])})
	}
	return defaults
}

func cloneGenerationDefaults(defaults []GenerationDefault) []GenerationDefault {
	cloned := make([]GenerationDefault, 0, len(defaults))
	for _, item := range defaults {
		cloned = append(cloned, GenerationDefault{Key: item.Key, Value: cloneRawMessage(item.Value)})
	}
	return cloned
}
