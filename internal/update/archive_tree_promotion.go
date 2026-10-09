package update

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func promoteArchiveTree(stagingPath string, targetPath string, binaryRelativePath string) (*installationPromotion, error) {
	stagedFiles, err := stagedArchiveFiles(stagingPath)
	if err != nil {
		return nil, err
	}
	stagedRelativePaths := make(map[string]struct{}, len(stagedFiles))
	for _, relativePath := range stagedFiles {
		stagedRelativePaths[normalizedInstallRelativePath(relativePath)] = struct{}{}
	}
	binaryRelativePath = filepath.FromSlash(binaryRelativePath)
	binaryKey := normalizedInstallRelativePath(binaryRelativePath)
	sort.Slice(stagedFiles, func(left int, right int) bool {
		leftIsBinary := normalizedInstallRelativePath(stagedFiles[left]) == binaryKey
		rightIsBinary := normalizedInstallRelativePath(stagedFiles[right]) == binaryKey
		if leftIsBinary != rightIsBinary {
			return !leftIsBinary
		}
		return stagedFiles[left] < stagedFiles[right]
	})
	group := &installationPromotion{}
	for _, relativePath := range stagedFiles {
		promotion, err := promoteBinary(filepath.Join(stagingPath, relativePath), filepath.Join(targetPath, relativePath))
		if err != nil {
			_ = group.Rollback()
			return nil, err
		}
		group.children = append(group.children, promotion)
	}
	if _, ok := stagedRelativePaths[binaryKey]; !ok {
		_ = group.Rollback()
		return nil, fmt.Errorf("staged archive does not contain executable %s", binaryRelativePath)
	}
	obsolete, err := obsoleteInstalledFiles(targetPath, stagedRelativePaths)
	if err != nil {
		_ = group.Rollback()
		return nil, err
	}
	group.obsolete = append(group.obsolete, obsolete...)
	return group, nil
}

func stagedArchiveFiles(stagingPath string) ([]string, error) {
	stagedFiles := make([]string, 0)
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
		return nil
	})
	return stagedFiles, err
}

func obsoleteInstalledFiles(targetPath string, stagedRelativePaths map[string]struct{}) ([]string, error) {
	info, err := os.Stat(targetPath)
	if err != nil || !info.IsDir() {
		return nil, nil
	}
	var obsolete []string
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
			obsolete = append(obsolete, path)
		}
		return nil
	})
	return obsolete, err
}

func binaryArchiveNames(target downloadTarget) []string {
	values := make([]string, 0, 4)
	for _, candidate := range []string{target.BinaryPath, target.Name} {
		value := strings.TrimSpace(filepath.Base(candidate))
		if value == "" || containsFold(values, value) {
			continue
		}
		values = append(values, value)
		if filepath.Ext(value) == "" && !containsFold(values, value+".exe") {
			values = append(values, value+".exe")
		}
	}
	return values
}

func containsFold(values []string, candidate string) bool {
	for _, value := range values {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}
