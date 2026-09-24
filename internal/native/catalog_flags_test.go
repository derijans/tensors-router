package native

import (
	"reflect"
	"strings"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cook"
)

func TestSDCPPCatalogNativeFlagsAreEmitted(t *testing.T) {
	for _, definition := range cook.OptionCatalog() {
		if definition.Lane != cook.LaneImage || definition.NativeFlag == "" {
			continue
		}
		if !containsString(definition.Backends, "llama_sdcpp") {
			continue
		}
		key := definition.Key
		t.Run(key, func(t *testing.T) {
			metadata := catalog.RuntimeConfig{SDModel: "C:/models/probe.safetensors"}
			field, ok := runtimeConfigField(&metadata, key)
			if !ok {
				t.Fatalf("catalog key %q has no matching catalog.RuntimeConfig field", key)
			}
			setProbeValue(t, key, field)
			args, err := RuntimeArgumentsForTest(metadata, "sdcpp")
			if err != nil {
				t.Fatal(err)
			}
			if !containsArgument(args, definition.NativeFlag) {
				t.Fatalf("option %q declares native flag %q but sdcppArguments never emits it: %#v", key, definition.NativeFlag, args)
			}
		})
	}
}

var catalogProbeStringValues = map[string]string{
	"load_mode": "mmap",
}

func TestLlamaCatalogNativeFlagsAreEmitted(t *testing.T) {
	baseline, err := RuntimeArgumentsForTest(catalog.RuntimeConfig{ModelParam: "C:/models/probe.gguf"}, "llama")
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range cook.OptionCatalog() {
		if !isLlamaServerCatalogFlag(definition) {
			continue
		}
		key := definition.Key
		t.Run(key, func(t *testing.T) {
			metadata := catalog.RuntimeConfig{ModelParam: "C:/models/probe.gguf"}
			field, ok := runtimeConfigField(&metadata, key)
			if !ok {
				if containsArgument(baseline, definition.NativeFlag) {
					return
				}
				t.Fatalf("catalog key %q declares llama-server flag %q but has no catalog.RuntimeConfig field", key, definition.NativeFlag)
			}
			setProbeValue(t, key, field)
			args, err := RuntimeArgumentsForTest(metadata, "llama")
			if err != nil {
				t.Fatal(err)
			}
			if !containsArgument(args, definition.NativeFlag) {
				t.Fatalf("option %q declares native flag %q but llamaArguments never emits it: %#v", key, definition.NativeFlag, args)
			}
		})
	}
}

func isLlamaServerCatalogFlag(definition cook.OptionDefinition) bool {
	if definition.NativeFlag == "" || !containsString(definition.Backends, "llama_sdcpp") {
		return false
	}
	if strings.HasPrefix(definition.Key, "llama_") {
		return false
	}
	switch definition.Lane {
	case cook.LaneText, cook.LaneMultimodal, cook.LaneEmbeddings, cook.LaneRuntime:
		return true
	default:
		return false
	}
}

func runtimeConfigField(metadata *catalog.RuntimeConfig, key string) (reflect.Value, bool) {
	structType := reflect.TypeOf(*metadata)
	for index := 0; index < structType.NumField(); index++ {
		if strings.Split(structType.Field(index).Tag.Get("json"), ",")[0] == key {
			return reflect.ValueOf(metadata).Elem().Field(index), true
		}
	}
	return reflect.Value{}, false
}

func setProbeValue(t *testing.T, key string, field reflect.Value) {
	t.Helper()
	if field.Kind() == reflect.Pointer {
		field.Set(reflect.New(field.Type().Elem()))
		field = field.Elem()
	}
	switch field.Kind() {
	case reflect.String:
		value, ok := catalogProbeStringValues[key]
		if !ok {
			value = "regression-test-value"
		}
		field.SetString(value)
	case reflect.Int:
		field.SetInt(7)
	case reflect.Float64:
		field.SetFloat(1.5)
	case reflect.Bool:
		field.SetBool(true)
	case reflect.Interface:
		field.Set(reflect.ValueOf("regression-test-value"))
	default:
		t.Fatalf("catalog key %q maps to unsupported field kind %s", key, field.Kind())
	}
}
