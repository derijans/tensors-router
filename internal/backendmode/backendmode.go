package backendmode

import (
	"fmt"
	"strings"
)

const (
	Key        = "backend_mode"
	Kobold     = "kobold"
	LlamaSDCPP = "llama_sdcpp"
)

func Normalize(value string) string {
	return strings.TrimSpace(value)
}

func Valid(value string) bool {
	switch Normalize(value) {
	case Kobold, LlamaSDCPP:
		return true
	default:
		return false
	}
}

func Resolve(value string, fallback string) (string, error) {
	value = Normalize(value)
	if value == "" {
		value = Normalize(fallback)
	}
	if Valid(value) {
		return value, nil
	}
	return "", fmt.Errorf("%s must be %s or %s", Key, Kobold, LlamaSDCPP)
}
