//go:build windows

package update

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var replaceFileProcedure = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")

func promoteBinary(stagingPath string, targetPath string) (*installationPromotion, error) {
	if err := syncFile(stagingPath); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return nil, err
	}
	backupPath := targetPath + ".previous"
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	_, statErr := os.Stat(targetPath)
	hadTarget := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, statErr
	}
	if !hadTarget {
		if err := os.Rename(stagingPath, targetPath); err != nil {
			return nil, err
		}
		return &installationPromotion{targetPath: targetPath, backupPath: backupPath}, nil
	}
	targetPointer, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return nil, err
	}
	stagingPointer, err := windows.UTF16PtrFromString(stagingPath)
	if err != nil {
		return nil, err
	}
	backupPointer, err := windows.UTF16PtrFromString(backupPath)
	if err != nil {
		return nil, err
	}
	result, _, callErr := replaceFileProcedure.Call(uintptr(unsafe.Pointer(targetPointer)), uintptr(unsafe.Pointer(stagingPointer)), uintptr(unsafe.Pointer(backupPointer)), 0, 0, 0)
	if result == 0 {
		if errors.Is(callErr, syscall.Errno(0)) {
			callErr = syscall.EINVAL
		}
		return nil, callErr
	}
	return &installationPromotion{targetPath: targetPath, backupPath: backupPath, hadTarget: true}, nil
}

func verifyExecutableMode(os.FileMode) error {
	return nil
}

func syncStagedHandle(*os.File) error {
	return nil
}
