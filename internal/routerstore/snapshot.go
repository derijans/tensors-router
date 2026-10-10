package routerstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

const snapshotFilePattern = ".snapshot-*.sqlite"

func (handle *Handle) Snapshot(ctx context.Context) (string, func(), error) {
	if handle == nil || handle.reader == nil {
		return "", nil, errors.New("router database is not open")
	}
	path, err := reserveAbsentFilename(filepath.Dir(handle.path), snapshotFilePattern)
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(path) }
	if _, err := handle.reader.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

func reserveAbsentFilename(directory string, pattern string) (string, error) {
	reserved, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	path := reserved.Name()
	if err := reserved.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, os.Remove(path)
}
