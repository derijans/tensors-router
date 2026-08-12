package vllm

import "golang.org/x/sys/windows"

func replaceFile(sourcePath string, destinationPath string) error {
	sourcePointer, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return err
	}
	destinationPointer, err := windows.UTF16PtrFromString(destinationPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(sourcePointer, destinationPointer, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func syncDirectory(string) error {
	return nil
}
