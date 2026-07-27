//go:build windows

package downloader

import "golang.org/x/sys/windows"

func availableSpace(directory string) (int64, bool, error) {
	var available uint64
	if err := windows.GetDiskFreeSpaceEx(windows.StringToUTF16Ptr(directory), &available, nil, nil); err != nil {
		return 0, false, err
	}
	if available > uint64(^uint64(0)>>1) {
		return int64(^uint64(0) >> 1), true, nil
	}
	return int64(available), true, nil
}
