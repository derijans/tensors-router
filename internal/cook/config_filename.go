package cook

import (
	"os"
	"path/filepath"
	"strings"
)

const configExtension = ".kcpps"

func ConfigFilenameOnDisk(configDir string, filename string) (string, error) {
	entries, err := os.ReadDir(configDir)
	if os.IsNotExist(err) {
		return filename, nil
	}
	if err != nil {
		return "", err
	}
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	sameStemFilename := ""
	for _, entry := range entries {
		name := entry.Name()
		if name == filename {
			return filename, nil
		}
		if sameStemFilename == "" && !entry.IsDir() && strings.TrimSuffix(name, filepath.Ext(name)) == stem && strings.EqualFold(filepath.Ext(name), configExtension) {
			sameStemFilename = name
		}
	}
	if sameStemFilename != "" {
		return sameStemFilename, nil
	}
	return filename, nil
}
