package vllm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

func stageEmbeddedUV(path string) (bool, error) {
	if len(embeddedUV) == 0 {
		return false, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return false, err
	}
	failed := true
	defer func() {
		_ = file.Close()
		if failed {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(embeddedUV); err != nil {
		return false, err
	}
	if err := file.Sync(); err != nil {
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	failed = false
	return true, nil
}

func EmbeddedUVBootstrap() (string, error) {
	if len(embeddedUV) == 0 {
		return "", fmt.Errorf("uv bootstrap is not embedded in this companion build")
	}
	digest := sha256.Sum256(embeddedUV)
	return hex.EncodeToString(digest[:]), nil
}

func embeddedUVAvailable() bool {
	return len(embeddedUV) > 0
}
