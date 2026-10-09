package downloader

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

type plannedTotals struct {
	files  []PlannedFile
	bytes  int64
	unsafe bool
}

type selectedArtifactKinds struct {
	gguf      bool
	weights   bool
	diffusers bool
}

func BuildPlan(details RepositoryDetails, requested []string, mode string, storageRoot string) (DownloadPlan, error) {
	if err := ValidateRepository(details.Repository); err != nil {
		return DownloadPlan{}, err
	}
	selected, err := selectedPlanPaths(details, requested, mode)
	if err != nil {
		return DownloadPlan{}, err
	}
	files, err := selectedPlannedFiles(details.Files, selected)
	if err != nil {
		return DownloadPlan{}, err
	}
	if mode != "explicit" {
		addSmartDependencies(details.Files, files, selected)
	}
	totals, err := orderedPlannedFiles(details.Files, files)
	if err != nil {
		return DownloadPlan{}, err
	}
	if first, second, collides := hostFileSystem.collision(plannedPaths(totals.files)); collides {
		return DownloadPlan{}, fmt.Errorf("files %q and %q differ only by letter case and would overwrite each other on case-insensitive file systems", first, second)
	}
	destination, err := planDestination(storageRoot, details, mode)
	if err != nil {
		return DownloadPlan{}, err
	}
	return DownloadPlan{
		Repository:    details.Repository,
		Revision:      details.Revision,
		Commit:        details.Commit,
		Files:         totals.files,
		TotalBytes:    totals.bytes,
		Destination:   destination,
		UnsafeWarning: totals.unsafe || (details.Security != "" && details.Security != "safe"),
		Gated:         details.Gated != "" && details.Gated != "false",
		Skipped:       details.Skipped,
		Snapshot:      mode == "snapshot",
	}, nil
}

func selectedPlanPaths(details RepositoryDetails, requested []string, mode string) (map[string]bool, error) {
	selected := map[string]bool{}
	for _, file := range requested {
		if err := ValidateRepositoryPath(file); err != nil {
			return nil, err
		}
		selected[file] = true
	}
	if mode == "snapshot" && len(details.Skipped) > 0 {
		return nil, fmt.Errorf("snapshot is incomplete because %q cannot be downloaded: %s", details.Skipped[0].Path, details.Skipped[0].Reason)
	}
	if mode == "snapshot" || len(selected) == 0 {
		for _, file := range details.Files {
			selected[file.Path] = true
		}
	}
	return selected, nil
}

func selectedPlannedFiles(repositoryFiles []File, selected map[string]bool) (map[string]PlannedFile, error) {
	files := map[string]PlannedFile{}
	for _, file := range repositoryFiles {
		if selected[file.Path] {
			if _, exists := files[file.Path]; !exists {
				files[file.Path] = plannedFile(file, false, "selected")
			}
		}
	}
	for file := range selected {
		if _, found := files[file]; !found {
			return nil, fmt.Errorf("file %q was not found in resolved repository", file)
		}
	}
	return files, nil
}

func plannedFile(file File, required bool, reason string) PlannedFile {
	return PlannedFile{Path: file.Path, Size: file.Size, Required: required, Reason: reason, LFSHash: file.LFSHash, GitOID: file.GitOID}
}

func orderedPlannedFiles(repositoryFiles []File, files map[string]PlannedFile) (plannedTotals, error) {
	totals := plannedTotals{files: make([]PlannedFile, 0, len(files))}
	for _, file := range repositoryFiles {
		planned, found := files[file.Path]
		if !found {
			continue
		}
		if file.Size < 0 {
			return plannedTotals{}, fmt.Errorf("file %q has invalid size", file.Path)
		}
		totals.bytes += file.Size
		totals.unsafe = totals.unsafe || file.Unsafe != "" && file.Unsafe != "safe"
		totals.files = append(totals.files, planned)
	}
	sort.Slice(totals.files, func(left int, right int) bool { return totals.files[left].Path < totals.files[right].Path })
	return totals, nil
}

func planDestination(storageRoot string, details RepositoryDetails, mode string) (string, error) {
	if mode == "snapshot" {
		return snapshotDirectoryResolve(storageRoot, details.Repository, details.Commit)
	}
	return repositoryDirectoryResolve(storageRoot, details.Repository)
}

func addSmartDependencies(repositoryFiles []File, planned map[string]PlannedFile, selected map[string]bool) {
	kinds := selectedKinds(selected)
	for _, file := range repositoryFiles {
		if _, exists := planned[file.Path]; exists {
			continue
		}
		if reason, needed := smartDependencyReason(file.Path, kinds, selected); needed {
			planned[file.Path] = plannedFile(file, true, reason)
		}
	}
}

func selectedKinds(selected map[string]bool) selectedArtifactKinds {
	var kinds selectedArtifactKinds
	for file := range selected {
		lower := strings.ToLower(file)
		kinds.gguf = kinds.gguf || strings.HasSuffix(lower, ".gguf")
		kinds.weights = kinds.weights || strings.HasSuffix(lower, ".safetensors") || strings.HasSuffix(lower, ".bin")
		kinds.diffusers = kinds.diffusers || strings.HasSuffix(lower, "model_index.json")
	}
	return kinds
}

func smartDependencyReason(filePath string, kinds selectedArtifactKinds, selected map[string]bool) (string, bool) {
	lower := strings.ToLower(path.Base(filePath))
	switch {
	case kinds.gguf && strings.HasSuffix(lower, ".gguf") && ggufShardForSelected(filePath, selected):
		return "GGUF shard set", true
	case kinds.gguf && strings.Contains(lower, "mmproj"):
		return "GGUF multimodal projector", true
	case kinds.weights && transformerSupportFile(lower):
		return "transformers runtime dependency", true
	case kinds.diffusers && (strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".safetensors")):
		return "diffusers component dependency", true
	default:
		return "", false
	}
}
