package native

import (
	"fmt"
	"strconv"
	"strings"

	"tensors-router/internal/catalog"
)

type launchTarget struct {
	modelID        string
	host           string
	port           string
	mcpServersPath string
	videoFFmpegDir string
}

type argumentBuilder func(catalog.RuntimeConfig, launchTarget) ([]string, error)

func appendStringArg(args *[]string, flag string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	*args = append(*args, flag, value)
}

func appendIntArg(args *[]string, flag string, value int) {
	if value == 0 {
		return
	}
	*args = append(*args, flag, strconv.Itoa(value))
}

func appendOptionalIntArg(args *[]string, flag string, value *int) {
	if value == nil {
		return
	}
	*args = append(*args, flag, strconv.Itoa(*value))
}

func appendFloatArg(args *[]string, flag string, value float64) {
	if value == 0 {
		return
	}
	*args = append(*args, flag, strconv.FormatFloat(value, 'f', -1, 64))
}

func appendFlag(args *[]string, flag string, enabled bool) {
	if enabled {
		*args = append(*args, flag)
	}
}

func appendOptionalBoolArg(args *[]string, enabledFlag string, disabledFlag string, value *bool) {
	if value == nil {
		return
	}
	if *value {
		*args = append(*args, enabledFlag)
		return
	}
	*args = append(*args, disabledFlag)
}

func appendOnOffArg(args *[]string, flag string, value *bool) {
	if value == nil {
		return
	}
	if *value {
		*args = append(*args, flag, "on")
		return
	}
	*args = append(*args, flag, "off")
}

func appendStringListArg(args *[]string, flag string, value any) {
	appendJoinedStringListArg(args, flag, value, ",")
}

func appendJoinedStringListArg(args *[]string, flag string, value any, separator string) {
	values := nativeStringValues(value)
	if len(values) == 0 {
		return
	}
	*args = append(*args, flag, strings.Join(values, separator))
}

func nativeStringValues(value any) []string {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, nativeStringValues(item)...)
		}
		return values
	case float64:
		return []string{strconv.FormatFloat(typed, 'f', -1, 64)}
	case int:
		return []string{strconv.Itoa(typed)}
	default:
		return nil
	}
}

func nativeSingleString(value any) string {
	values := nativeStringValues(value)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func positive(value int) int {
	if value > 0 {
		return value
	}
	return 0
}

func nonNegative(value int) int {
	if value >= 0 {
		return value
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func RuntimeArgumentsForTest(metadata catalog.RuntimeConfig, kind string) ([]string, error) {
	switch kind {
	case "llama":
		return llamaArguments(metadata, launchTarget{modelID: "model", host: "127.0.0.1", port: "5002"})
	case "sdcpp":
		return sdcppArguments(metadata, launchTarget{modelID: "model", host: "127.0.0.1", port: "7860"})
	case "whispercpp":
		return whisperCPPArguments(metadata, launchTarget{modelID: "model", host: "127.0.0.1", port: "5003"})
	default:
		return nil, fmt.Errorf("unknown native server kind %q", kind)
	}
}
