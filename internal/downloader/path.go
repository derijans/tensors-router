package downloader

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func RepositoryDirectory(root string, repository string) (string, error) {
	owner, name, err := splitRepository(repository)
	if err != nil {
		return "", err
	}
	return secureJoin(root, owner, name)
}

func DestinationPath(root string, repository string, repositoryPath string) (string, error) {
	directory, err := RepositoryDirectory(root, repository)
	if err != nil {
		return "", err
	}
	parts, err := safeRepositoryPath(repositoryPath)
	if err != nil {
		return "", err
	}
	return secureJoin(directory, parts...)
}

func ValidateRepository(repository string) error {
	_, _, err := splitRepository(repository)
	return err
}

func ValidateRepositoryPath(repositoryPath string) error {
	_, err := safeRepositoryPath(repositoryPath)
	return err
}

func splitRepository(repository string) (string, string, error) {
	repository = strings.TrimSpace(repository)
	owner, name, found := strings.Cut(repository, "/")
	if !found || strings.Contains(name, "/") || !safeRepositoryPart(owner) || !safeRepositoryPart(name) {
		return "", "", fmt.Errorf("repository must have a safe owner/name form")
	}
	return owner, name, nil
}

func safeRepositoryPart(value string) bool {
	if value == "" || value == "." || value == ".." || len(value) > 256 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func safeRepositoryPath(repositoryPath string) ([]string, error) {
	repositoryPath = strings.TrimSpace(repositoryPath)
	if repositoryPath == "" || strings.Contains(repositoryPath, "\\") || path.IsAbs(repositoryPath) || filepath.IsAbs(repositoryPath) {
		return nil, fmt.Errorf("repository path is invalid")
	}
	cleaned := path.Clean(repositoryPath)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return nil, fmt.Errorf("repository path escapes its repository")
	}
	parts := strings.Split(cleaned, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsRune(part, 0) {
			return nil, fmt.Errorf("repository path is invalid")
		}
	}
	return parts, nil
}

func secureJoin(root string, parts ...string) (string, error) {
	root, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	if err := ensureDirectory(root); err != nil {
		return "", err
	}
	current := root
	for index, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, `/\\`) {
			return "", fmt.Errorf("path component is invalid")
		}
		current = filepath.Join(current, part)
		if index < len(parts)-1 {
			if err := ensureDirectory(current); err != nil {
				return "", err
			}
			continue
		}
		if info, err := os.Lstat(current); err == nil && info.Mode()&fs.ModeSymlink != 0 {
			return "", fmt.Errorf("destination is a symbolic link")
		} else if err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}
	if !pathWithin(current, root) {
		return "", fmt.Errorf("destination escapes storage root")
	}
	return current, nil
}

func ensureDirectory(directory string) error {
	if info, err := os.Lstat(directory); err == nil {
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%s is not a safe directory", directory)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Mkdir(directory, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s is not a safe directory", directory)
	}
	return nil
}
