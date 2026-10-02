package downloader

import (
	"fmt"
	"runtime"
	"strings"
)

type fileSystemRules struct {
	rejectsWindowsNames bool
	caseInsensitive     bool
}

var hostFileSystem = fileSystemRules{
	rejectsWindowsNames: runtime.GOOS == "windows",
	caseInsensitive:     runtime.GOOS == "windows" || runtime.GOOS == "darwin",
}

var reservedDeviceNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true, "CONIN$": true, "CONOUT$": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

func (rules fileSystemRules) nameProblem(name string) string {
	for _, character := range name {
		if character < 0x20 || character == 0x7f {
			return "contains a control character"
		}
	}
	if !rules.rejectsWindowsNames {
		return ""
	}
	if index := strings.IndexAny(name, `<>:"|?*`); index >= 0 {
		return fmt.Sprintf("contains %q, which Windows file systems reject or treat as a stream separator", name[index])
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return "ends with a dot or space, which Windows silently strips"
	}
	stem, _, _ := strings.Cut(name, ".")
	if reservedDeviceNames[strings.ToUpper(strings.TrimRight(stem, " "))] {
		return "is a reserved Windows device name"
	}
	return ""
}

func (rules fileSystemRules) collision(paths []string) (string, string, bool) {
	if !rules.caseInsensitive {
		return "", "", false
	}
	seen := make(map[string]string, len(paths))
	for _, path := range paths {
		folded := strings.ToLower(path)
		if previous, found := seen[folded]; found && previous != path {
			return previous, path, true
		}
		seen[folded] = path
	}
	return "", "", false
}
