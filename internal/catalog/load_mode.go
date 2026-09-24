package catalog

import (
	"fmt"
	"slices"
	"strings"
)

var llamaLoadModes = []string{"auto", "none", "mmap", "mlock", "mmap+mlock", "dio"}

func LlamaLoadModes() []string {
	return slices.Clone(llamaLoadModes)
}

func (metadata RuntimeConfig) LlamaLoadMode() (string, error) {
	mode := strings.TrimSpace(metadata.LoadMode)
	if mode == "" {
		return legacyLlamaLoadMode(metadata.UseMMap, metadata.UseMLock), nil
	}
	if !slices.Contains(llamaLoadModes, mode) {
		return "", fmt.Errorf("load_mode %q is not one of %s", mode, strings.Join(llamaLoadModes, ", "))
	}
	return mode, nil
}

func legacyLlamaLoadMode(useMMap bool, useMLock bool) string {
	switch {
	case useMMap && useMLock:
		return "mmap+mlock"
	case useMMap:
		return "mmap"
	case useMLock:
		return "mlock"
	default:
		return "none"
	}
}
