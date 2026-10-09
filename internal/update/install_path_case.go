package update

import (
	"path/filepath"
	"runtime"
	"strings"
)

var installPathsFoldCase = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

func normalizedInstallRelativePath(path string) string {
	cleanPath := filepath.Clean(path)
	if installPathsFoldCase {
		return strings.ToLower(cleanPath)
	}
	return cleanPath
}

func sameInstallPath(left string, right string) bool {
	if installPathsFoldCase {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func trimInstallPathSuffix(value string, suffix string) (string, bool) {
	if len(value) < len(suffix) || !sameInstallPath(value[len(value)-len(suffix):], suffix) {
		return value, false
	}
	return value[:len(value)-len(suffix)], true
}
