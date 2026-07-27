package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func SHA256File(filePath string) (string, int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func HashSidecarPath(filePath string) string {
	extension := filepath.Ext(filePath)
	if extension == "" {
		return filePath + ".hash"
	}
	return strings.TrimSuffix(filePath, extension) + ".hash"
}

func ReadTrustedHashSidecar(filePath string) (string, bool, error) {
	sidecarPath := HashSidecarPath(filePath)
	content, err := os.ReadFile(sidecarPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	value := strings.TrimSpace(string(content))
	if !validSHA256(value) {
		return "", false, nil
	}
	ambiguous, err := sameStemCollision(filePath)
	if err != nil {
		return "", false, err
	}
	return value, !ambiguous, nil
}

func WriteHashSidecar(filePath string, hash string) error {
	if !validSHA256(hash) {
		return fmt.Errorf("hash must be a lowercase SHA-256 value")
	}
	ambiguous, err := sameStemCollision(filePath)
	if err != nil {
		return err
	}
	if ambiguous {
		return nil
	}
	return os.WriteFile(HashSidecarPath(filePath), []byte(hash+"\n"), 0o600)
}

func validSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sameStemCollision(filePath string) (bool, error) {
	directory := filepath.Dir(filePath)
	name := filepath.Base(filePath)
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false, err
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || entry.Name() == filepath.Base(HashSidecarPath(filePath)) {
			continue
		}
		if strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())) == stem {
			count++
		}
	}
	return count > 1, nil
}
