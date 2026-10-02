//go:build windows

package downloader

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

const renameRetryWindow = 10 * time.Second

func renameWithRetry(from string, to string) error {
	deadline := time.Now().Add(renameRetryWindow)
	delay := 50 * time.Millisecond
	for {
		err := os.Rename(from, to)
		if err == nil || !transientlyLocked(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(delay)
		delay = min(delay*2, time.Second)
	}
}

func transientlyLocked(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
