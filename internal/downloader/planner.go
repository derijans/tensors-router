package downloader

import (
	"path"
	"strings"
)

func plannedPaths(files []PlannedFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

func ggufShardForSelected(candidate string, selected map[string]bool) bool {
	if selected[candidate] {
		return true
	}
	base := path.Base(candidate)
	for value := range selected {
		selectedBase := path.Base(value)
		prefix := strings.Split(selectedBase, "-000")[0]
		if prefix != selectedBase && strings.HasPrefix(base, prefix+"-") {
			return true
		}
	}
	return false
}

func transformerSupportFile(name string) bool {
	if name == "config.json" || name == "generation_config.json" || name == "chat_template.json" || name == "preprocessor_config.json" || name == "processor_config.json" || name == "special_tokens_map.json" {
		return true
	}
	return strings.HasPrefix(name, "tokenizer") || strings.HasSuffix(name, ".safetensors.index.json")
}
