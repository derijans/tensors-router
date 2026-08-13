//go:build !windows

package vllm

import "os"

func replaceFile(sourcePath string, destinationPath string) error {
	return os.Rename(sourcePath, destinationPath)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
