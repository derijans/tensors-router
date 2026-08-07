//go:build !windows

package update

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

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
	if hadTarget {
		if err := os.Link(targetPath, backupPath); err != nil {
			if err := copyBackup(targetPath, backupPath); err != nil {
				return nil, err
			}
		}
	}
	if err := os.Rename(stagingPath, targetPath); err != nil {
		if hadTarget {
			_ = os.Remove(backupPath)
		}
		return nil, err
	}
	return &installationPromotion{targetPath: targetPath, backupPath: backupPath, hadTarget: hadTarget}, nil
}

func copyBackup(sourcePath string, backupPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	backup, err := os.OpenFile(backupPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(backup, source)
	syncErr := backup.Sync()
	closeErr := backup.Close()
	if copyErr != nil {
		_ = os.Remove(backupPath)
		return copyErr
	}
	if syncErr != nil {
		_ = os.Remove(backupPath)
		return syncErr
	}
	if closeErr != nil {
		_ = os.Remove(backupPath)
		return closeErr
	}
	return nil
}

func verifyExecutableMode(mode os.FileMode) error {
	if mode.Perm()&0o111 == 0 {
		return fmt.Errorf("promoted executable lacks execute permission")
	}
	return nil
}

func syncStagedHandle(file *os.File) error {
	return file.Sync()
}
