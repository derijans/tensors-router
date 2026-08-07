package credential

import (
	"fmt"
	"os"
	"strings"
)

const maxFileBytes = 16 * 1024

type Source struct {
	Role      string
	ValueName string
	Value     string
	FileName  string
	FilePath  string
}

func Resolve(source Source) (string, bool, error) {
	value := strings.TrimSpace(source.Value)
	filePath := strings.TrimSpace(source.FilePath)
	if value != "" && filePath != "" {
		return "", false, fmt.Errorf("%s: %s and %s are mutually exclusive", source.Role, source.ValueName, source.FileName)
	}
	if filePath == "" {
		if value == "" {
			return "", false, nil
		}
		if err := validateScalar(source.Role, source.ValueName, source.Value); err != nil {
			return "", false, err
		}
		return value, true, nil
	}
	info, err := os.Lstat(filePath)
	if err != nil {
		return "", false, fmt.Errorf("%s: read %s: %w", source.Role, source.FileName, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", false, fmt.Errorf("%s: %s cannot reference a symbolic link", source.Role, source.FileName)
	}
	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("%s: %s must reference a regular file", source.Role, source.FileName)
	}
	if info.Size() > maxFileBytes {
		return "", false, fmt.Errorf("%s: %s exceeds %d bytes", source.Role, source.FileName, maxFileBytes)
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", false, fmt.Errorf("%s: read %s: %w", source.Role, source.FileName, err)
	}
	value = strings.TrimSpace(string(content))
	if value == "" {
		return "", false, fmt.Errorf("%s: %s is empty", source.Role, source.FileName)
	}
	if err := validateScalar(source.Role, source.FileName, value); err != nil {
		return "", false, err
	}
	return value, true, nil
}

func validateScalar(role string, sourceName string, value string) error {
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s: %s must contain one credential", role, sourceName)
	}
	return nil
}
