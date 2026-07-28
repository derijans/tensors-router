package companion

import (
	"os"
	"path/filepath"
	"strings"
)

func FindSibling(executablePath string, targetStem string, sourceStems ...string) (string, bool) {
	for _, candidate := range siblingCandidates(executablePath, targetStem, sourceStems...) {
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() {
			return candidate, true
		}
	}
	return "", false
}

func PreferredSibling(executablePath string, targetStem string, sourceStems ...string) string {
	candidates := siblingCandidates(executablePath, targetStem, sourceStems...)
	if len(candidates) == 0 {
		return filepath.Join(filepath.Dir(executablePath), targetStem)
	}
	return candidates[0]
}

func siblingCandidates(executablePath string, targetStem string, sourceStems ...string) []string {
	directory := filepath.Dir(executablePath)
	name := filepath.Base(executablePath)
	extension := filepath.Ext(name)
	currentStem := strings.TrimSuffix(name, extension)
	candidates := make([]string, 0, 2)
	for _, sourceStem := range sourceStems {
		prefix := sourceStem + "-"
		if strings.HasPrefix(currentStem, prefix) {
			candidates = append(candidates, filepath.Join(directory, targetStem+currentStem[len(sourceStem):]+extension))
			break
		}
	}
	candidates = append(candidates, filepath.Join(directory, targetStem+extension))
	return candidates
}
