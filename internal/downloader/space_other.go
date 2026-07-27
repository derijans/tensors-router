//go:build !linux && !windows

package downloader

func availableSpace(string) (int64, bool, error) { return 0, false, nil }
