//go:build linux

package downloader

import "syscall"

func availableSpace(directory string) (int64, bool, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(directory, &stat); err != nil {
		return 0, false, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), true, nil
}
