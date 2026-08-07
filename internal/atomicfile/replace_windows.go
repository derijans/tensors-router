//go:build windows

package atomicfile

import (
	"errors"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var replaceFileProcedure = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")

func replace(source string, target string) error {
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return os.Rename(source, target)
	} else if err != nil {
		return err
	}
	targetPointer, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	sourcePointer, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	result, _, callErr := replaceFileProcedure.Call(uintptr(unsafe.Pointer(targetPointer)), uintptr(unsafe.Pointer(sourcePointer)), 0, 0, 0, 0)
	if result != 0 {
		return nil
	}
	if errors.Is(callErr, syscall.Errno(0)) {
		return syscall.EINVAL
	}
	return callErr
}
