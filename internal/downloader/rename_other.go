//go:build !windows

package downloader

import "os"

func renameWithRetry(from string, to string) error {
	return os.Rename(from, to)
}
