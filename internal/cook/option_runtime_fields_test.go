package cook

import (
	"reflect"
	"strings"
	"testing"

	"tensors-router/internal/catalog"
)

type runtimeConfigFieldShape struct {
	kind     reflect.Kind
	elemKind reflect.Kind
}

func runtimeConfigFieldShapes() map[string]runtimeConfigFieldShape {
	shapes := map[string]runtimeConfigFieldShape{}
	structType := reflect.TypeOf(catalog.RuntimeConfig{})
	for index := 0; index < structType.NumField(); index++ {
		field := structType.Field(index)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		fieldType := field.Type
		for fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		shape := runtimeConfigFieldShape{kind: fieldType.Kind()}
		if shape.kind == reflect.Slice || shape.kind == reflect.Array {
			shape.elemKind = fieldType.Elem().Kind()
		}
		shapes[name] = shape
	}
	return shapes
}

func valueTypeAcceptsShape(valueType string, shape runtimeConfigFieldShape) bool {
	if shape.kind == reflect.Interface {
		return true
	}
	switch valueType {
	case ValueNumber:
		switch shape.kind {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			return true
		}
		return false
	case ValueBool:
		return shape.kind == reflect.Bool
	case ValueString:
		return shape.kind == reflect.String
	case ValueStringList:
		return (shape.kind == reflect.Slice || shape.kind == reflect.Array) && shape.elemKind == reflect.String
	case ValueJSON:
		switch shape.kind {
		case reflect.Map, reflect.Slice, reflect.Struct:
			return true
		}
		return false
	default:
		return false
	}
}

func runtimeConfigKeyForOption(key string) string {
	return strings.TrimPrefix(key, "llama_")
}

var optionsNotDecodedByRouterReasons = map[string]string{
	"baseconfig":             "koboldcpp base config path, passed through untouched",
	"config":                 "koboldcpp config path, passed through untouched",
	"host":                   "koboldcpp server bind host",
	"port":                   "koboldcpp server bind port",
	"quiet":                  "koboldcpp server flag",
	"launch":                 "koboldcpp server flag",
	"showgui":                "koboldcpp server flag",
	"skiplauncher":           "koboldcpp server flag",
	"admin":                  "koboldcpp server flag",
	"adminpassword":          "koboldcpp server option",
	"admindir":               "koboldcpp server option",
	"adminunloadtimeout":     "koboldcpp server option",
	"routermode":             "koboldcpp server flag",
	"autoswapmode":           "koboldcpp server flag",
	"reqtimeout":             "koboldcpp server option",
	"password":               "koboldcpp server option",
	"ssl":                    "koboldcpp server option",
	"nocertify":              "koboldcpp server flag",
	"remotetunnel":           "koboldcpp server flag",
	"multiuser":              "koboldcpp server option",
	"multiplayer":            "koboldcpp server flag",
	"websearch":              "koboldcpp server flag",
	"maxrequestsize":         "koboldcpp server option",
	"onready":                "koboldcpp server option",
	"preloadstory":           "koboldcpp server option",
	"savedatafile":           "koboldcpp server option",
	"mcp_servers":            "koboldcpp server option",
	"downloaddir":            "koboldcpp server option",
	"parallelrequests":       "koboldcpp server option",
	"gendefaults":            "koboldcpp server option",
	"gendefaultsoverwrite":   "koboldcpp server flag",
	"defaultgenamt":          "koboldcpp server option",
	"genlimit":               "koboldcpp server option",
	"promptlimit":            "koboldcpp server option",
	"ratelimit":              "koboldcpp server option",
	"debugmode":              "koboldcpp server option",
	"rpcmode":                "koboldcpp server option",
	"rpcport":                "koboldcpp server option",
	"rpchost":                "koboldcpp server option",
	"rpcdevice":              "koboldcpp server option",
	"noflashattention":       "koboldcpp text option",
	"lowvram":                "koboldcpp text option",
	"nommq":                  "koboldcpp text option",
	"autofit":                "koboldcpp text option",
	"blasbatchsize":          "koboldcpp text option",
	"reasoningeffort":        "koboldcpp text option, distinct from the llama.cpp reasoning_effort field",
	"usemtp":                 "koboldcpp text option",
	"swapadding":             "koboldcpp text option",
	"useswa":                 "koboldcpp text option",
	"noswa":                  "koboldcpp text option",
	"smartcache":             "koboldcpp text option",
	"smartcontext":           "koboldcpp text option",
	"contbatch":              "koboldcpp text option, distinct from the llama.cpp cont_batching field",
	"pipelineparallel":       "koboldcpp text option",
	"nopipelineparallel":     "koboldcpp text option",
	"chatcompletionsadapter": "koboldcpp text option",
	"moecpu":                 "koboldcpp text option",
	"moeexperts":             "koboldcpp text option",
	"nobostoken":             "koboldcpp text option",
	"ropeconfig":             "koboldcpp text option",
	"overridenativecontext":  "koboldcpp text option",
	"overridekv":             "koboldcpp text option",
	"overridetensors":        "koboldcpp text option",
	"lora":                   "koboldcpp text option",
	"loramult":               "koboldcpp text option",
	"draftgpusplit":          "koboldcpp text option",
	"jinja":                  "normalized by catalog.NormalizeJinjaKwargs, not a RuntimeConfig field",
	"jinja_tools":            "normalized by catalog.NormalizeJinjaKwargs, not a RuntimeConfig field",
	"jinja_kwargs":           "normalized by catalog.NormalizeJinjaKwargs, not a RuntimeConfig field",
	"jinjatemplate":          "normalized by catalog.NormalizeJinjaKwargs, not a RuntimeConfig field",
	"jinjathink":             "normalized by catalog.NormalizeJinjaKwargs, not a RuntimeConfig field",
	"pooling":                "llama.cpp text option",
	"sdloramult":             "koboldcpp image option",
	"sdmaingpu":              "koboldcpp image option",
	"sdvaedevice":            "koboldcpp image option",
	"sdclipdevice":           "koboldcpp image option",
	"sdconvdirect":           "koboldcpp image option",
	"sdvramlimit":            "koboldcpp image option",
	"sdclamped":              "koboldcpp image option",
	"sdclampedsoft":          "koboldcpp image option",
	"enableguidance":         "koboldcpp image option",
	"sdgendefaults":          "koboldcpp image option",
	"sdconfig":               "koboldcpp image option",
}

func TestOptionValueTypesMatchRuntimeConfigFields(t *testing.T) {
	shapes := runtimeConfigFieldShapes()
	for _, definition := range OptionCatalog() {
		key := runtimeConfigKeyForOption(definition.Key)
		if strings.HasPrefix(key, "whispercpp_") && key != "whispercpp_vad_model" {
			continue
		}
		shape, ok := shapes[key]
		if !ok {
			continue
		}
		if !valueTypeAcceptsShape(definition.ValueType, shape) {
			t.Errorf("%s: value_type %q cannot decode into catalog.RuntimeConfig field of kind %s", definition.Key, definition.ValueType, shape.kind)
		}
	}
}

func TestRuntimeConfigFieldsCoverNonPassthroughOptions(t *testing.T) {
	shapes := runtimeConfigFieldShapes()
	seen := map[string]bool{}
	for _, definition := range OptionCatalog() {
		key := runtimeConfigKeyForOption(definition.Key)
		if seen[key] {
			continue
		}
		seen[key] = true
		if strings.HasPrefix(key, "whispercpp_") && key != "whispercpp_vad_model" {
			continue
		}
		_, hasField := shapes[key]
		reason, allowlisted := optionsNotDecodedByRouterReasons[key]
		if hasField && allowlisted {
			t.Errorf("%s: listed in optionsNotDecodedByRouterReasons (%q) but catalog.RuntimeConfig now has a field for it; remove the allowlist entry", key, reason)
		}
		if !hasField && !allowlisted {
			t.Errorf("%s: has no catalog.RuntimeConfig field and is not in optionsNotDecodedByRouterReasons; add a field or document why it is a passthrough", key)
		}
	}
}

func TestLlamaAliasOptionsShareBaseValueType(t *testing.T) {
	byKey := map[string]OptionDefinition{}
	for _, definition := range OptionCatalog() {
		byKey[definition.Key] = definition
	}
	for key, definition := range byKey {
		if !strings.HasPrefix(key, "llama_") {
			continue
		}
		base, ok := byKey[strings.TrimPrefix(key, "llama_")]
		if !ok {
			t.Errorf("%s: alias has no base option %q", key, strings.TrimPrefix(key, "llama_"))
			continue
		}
		if definition.ValueType != base.ValueType {
			t.Errorf("%s: value_type %q diverges from base option %q value_type %q", key, definition.ValueType, base.Key, base.ValueType)
		}
	}
}

func TestWhisperCPPOptionsAreCollectedByRuntimeConfigLoader(t *testing.T) {
	body := map[string]any{"model_param": "text.gguf"}
	for _, definition := range OptionCatalog() {
		if !strings.HasPrefix(definition.Key, "whispercpp_") {
			continue
		}
		switch definition.ValueType {
		case ValueBool:
			body[definition.Key] = true
		case ValueNumber:
			body[definition.Key] = 1
		default:
			body[definition.Key] = "value"
		}
	}
	content := rawJSON(t, body)
	metadata, err := catalog.DecodeRuntimeConfig(content)
	if err != nil {
		t.Fatalf("DecodeRuntimeConfig failed: %v", err)
	}
	for _, definition := range OptionCatalog() {
		if !strings.HasPrefix(definition.Key, "whispercpp_") || definition.Key == "whispercpp_vad_model" {
			continue
		}
		if _, ok := metadata.WhisperCPPOptions[definition.Key]; !ok {
			t.Errorf("%s: not collected into RuntimeConfig.WhisperCPPOptions", definition.Key)
		}
	}
}
