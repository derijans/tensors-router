package downloader

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

func BuildPlan(details RepositoryDetails, requested []string, mode string, storageRoot string) (DownloadPlan, error) {
	if err := ValidateRepository(details.Repository); err != nil {
		return DownloadPlan{}, err
	}
	selected := map[string]bool{}
	for _, file := range requested {
		if err := ValidateRepositoryPath(file); err != nil {
			return DownloadPlan{}, err
		}
		selected[file] = true
	}
	if mode == "snapshot" || len(selected) == 0 {
		for _, file := range details.Files {
			selected[file.Path] = true
		}
	}
	files := map[string]PlannedFile{}
	add := func(file File, required bool, reason string) {
		current, exists := files[file.Path]
		if !exists || (required && !current.Required) {
			files[file.Path] = PlannedFile{Path: file.Path, Size: file.Size, Required: required, Reason: reason, LFSHash: file.LFSHash}
		}
	}
	for _, file := range details.Files {
		if selected[file.Path] {
			add(file, false, "selected")
		}
	}
	if len(files) != len(selected) {
		for file := range selected {
			if _, found := files[file]; !found {
				return DownloadPlan{}, fmt.Errorf("file %q was not found in resolved repository", file)
			}
		}
	}
	if mode != "explicit" {
		addSmartDependencies(details.Files, files, selected)
	}
	planned := make([]PlannedFile, 0, len(files))
	var total int64
	unsafe := false
	for _, file := range details.Files {
		plannedFile, found := files[file.Path]
		if !found {
			continue
		}
		if file.Size < 0 {
			return DownloadPlan{}, fmt.Errorf("file %q has invalid size", file.Path)
		}
		total += file.Size
		if file.Unsafe != "" && file.Unsafe != "safe" {
			unsafe = true
		}
		planned = append(planned, plannedFile)
	}
	sort.Slice(planned, func(left int, right int) bool { return planned[left].Path < planned[right].Path })
	destination, err := RepositoryDirectory(storageRoot, details.Repository)
	if mode == "snapshot" {
		destination, err = SnapshotDirectory(storageRoot, details.Repository, details.Commit)
	}
	if err != nil {
		return DownloadPlan{}, err
	}
	gated := details.Gated != "" && details.Gated != "false"
	return DownloadPlan{Repository: details.Repository, Revision: details.Revision, Commit: details.Commit, Files: planned, TotalBytes: total, Destination: destination, UnsafeWarning: gated || unsafe || (details.Security != "" && details.Security != "safe"), Snapshot: mode == "snapshot"}, nil
}

func addSmartDependencies(repositoryFiles []File, planned map[string]PlannedFile, selected map[string]bool) {
	selectedGGUF := false
	selectedWeights := false
	selectedDiffusers := false
	for file := range selected {
		lower := strings.ToLower(file)
		selectedGGUF = selectedGGUF || strings.HasSuffix(lower, ".gguf")
		selectedWeights = selectedWeights || strings.HasSuffix(lower, ".safetensors") || strings.HasSuffix(lower, ".bin")
		selectedDiffusers = selectedDiffusers || strings.HasSuffix(lower, "model_index.json")
	}
	add := func(file File, reason string) {
		if _, exists := planned[file.Path]; !exists {
			planned[file.Path] = PlannedFile{Path: file.Path, Size: file.Size, Required: true, Reason: reason, LFSHash: file.LFSHash}
		}
	}
	for _, file := range repositoryFiles {
		lower := strings.ToLower(path.Base(file.Path))
		if selectedGGUF && strings.HasSuffix(lower, ".gguf") && ggufShardForSelected(file.Path, selected) {
			add(file, "GGUF shard set")
		}
		if selectedGGUF && strings.Contains(lower, "mmproj") {
			add(file, "GGUF multimodal projector")
		}
		if selectedWeights && transformerSupportFile(lower) {
			add(file, "transformers runtime dependency")
		}
		if selectedDiffusers && (strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".safetensors")) {
			add(file, "diffusers component dependency")
		}
	}
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
