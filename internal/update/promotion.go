package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type installationPromotion struct {
	targetPath string
	backupPath string
	hadTarget  bool
	directory  bool
	children   []*installationPromotion
	obsolete   []string
}

func promoteDirectory(stagingPath string, targetPath string) (*installationPromotion, error) {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return nil, err
	}
	backupPath := targetPath + ".previous"
	if err := removeInstallDir(backupPath); err != nil {
		return nil, err
	}
	_, statErr := os.Stat(targetPath)
	hadTarget := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, statErr
	}
	if hadTarget {
		if err := os.Rename(targetPath, backupPath); err != nil {
			return nil, err
		}
	}
	if err := os.Rename(stagingPath, targetPath); err != nil {
		if hadTarget {
			if rollbackErr := os.Rename(backupPath, targetPath); rollbackErr != nil {
				return nil, fmt.Errorf("install failed: %v; rollback failed: %w", err, rollbackErr)
			}
		}
		return nil, err
	}
	return &installationPromotion{targetPath: targetPath, backupPath: backupPath, hadTarget: hadTarget, directory: true}, nil
}

func (promotion *installationPromotion) Commit() error {
	if promotion == nil {
		return nil
	}
	if len(promotion.children) > 0 {
		errorsFound := make([]error, 0)
		for _, child := range promotion.children {
			if err := child.Commit(); err != nil {
				errorsFound = append(errorsFound, err)
			}
		}
		for _, obsoletePath := range promotion.obsolete {
			if err := os.Remove(obsoletePath); err != nil && !os.IsNotExist(err) {
				errorsFound = append(errorsFound, err)
			}
		}
		return errors.Join(errorsFound...)
	}
	if promotion == nil || !promotion.hadTarget {
		return nil
	}
	if promotion.directory {
		return removeInstallDir(promotion.backupPath)
	}
	return os.Remove(promotion.backupPath)
}

func (promotion *installationPromotion) Rollback() error {
	if promotion == nil {
		return nil
	}
	if len(promotion.children) > 0 {
		errorsFound := make([]error, 0)
		for index := len(promotion.children) - 1; index >= 0; index-- {
			if err := promotion.children[index].Rollback(); err != nil {
				errorsFound = append(errorsFound, err)
			}
		}
		return errors.Join(errorsFound...)
	}
	if promotion.directory {
		if err := removeInstallDir(promotion.targetPath); err != nil {
			return err
		}
	} else if err := os.Remove(promotion.targetPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if !promotion.hadTarget {
		return nil
	}
	return os.Rename(promotion.backupPath, promotion.targetPath)
}

func verifyPromotedBinary(path string, expectedSHA256 string) error {
	actualSHA256, err := fileSHA256Hex(path)
	if err != nil {
		return err
	}
	if actualSHA256 != expectedSHA256 {
		return fmt.Errorf("promoted executable SHA-256 mismatch")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("promoted executable is not a regular file")
	}
	return verifyExecutableMode(info.Mode())
}
func syncFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return syncStagedHandle(file)
}
