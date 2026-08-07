package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type installationPromotion struct {
	targetPath string
	backupPath string
	hadTarget  bool
	directory  bool
	children   []*installationPromotion
	obsolete   []string
}

func promoteArchiveTree(stagingPath string, targetPath string, binaryRelativePath string) (*installationPromotion, error) {
	stagedFiles := make([]string, 0)
	stagedRelativePaths := map[string]struct{}{}
	err := filepath.WalkDir(stagingPath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("staged archive contains unsupported entry %s", path)
		}
		relativePath, err := filepath.Rel(stagingPath, path)
		if err != nil {
			return err
		}
		stagedFiles = append(stagedFiles, relativePath)
		stagedRelativePaths[normalizedInstallRelativePath(relativePath)] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, err
	}
	binaryRelativePath = filepath.FromSlash(binaryRelativePath)
	sort.Slice(stagedFiles, func(left int, right int) bool {
		leftIsBinary := normalizedInstallRelativePath(stagedFiles[left]) == normalizedInstallRelativePath(binaryRelativePath)
		rightIsBinary := normalizedInstallRelativePath(stagedFiles[right]) == normalizedInstallRelativePath(binaryRelativePath)
		if leftIsBinary != rightIsBinary {
			return !leftIsBinary
		}
		return stagedFiles[left] < stagedFiles[right]
	})
	group := &installationPromotion{}
	for _, relativePath := range stagedFiles {
		stagedFile := filepath.Join(stagingPath, relativePath)
		targetFile := filepath.Join(targetPath, relativePath)
		promotion, err := promoteBinary(stagedFile, targetFile)
		if err != nil {
			_ = group.Rollback()
			return nil, err
		}
		group.children = append(group.children, promotion)
	}
	if _, ok := stagedRelativePaths[normalizedInstallRelativePath(binaryRelativePath)]; !ok {
		_ = group.Rollback()
		return nil, fmt.Errorf("staged archive does not contain executable %s", binaryRelativePath)
	}
	if info, err := os.Stat(targetPath); err == nil && info.IsDir() {
		err = filepath.WalkDir(targetPath, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || strings.HasSuffix(entry.Name(), ".previous") {
				return nil
			}
			relativePath, err := filepath.Rel(targetPath, path)
			if err != nil {
				return err
			}
			if _, ok := stagedRelativePaths[normalizedInstallRelativePath(relativePath)]; !ok {
				group.obsolete = append(group.obsolete, path)
			}
			return nil
		})
		if err != nil {
			_ = group.Rollback()
			return nil, err
		}
	}
	return group, nil
}

func normalizedInstallRelativePath(path string) string {
	return strings.ToLower(filepath.Clean(path))
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
