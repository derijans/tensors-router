package routerstore

import (
	"fmt"
	"os"
	"time"
)

func archiveLegacyFile(path string) error {
	destination, err := archiveDestination(path)
	if err != nil {
		return err
	}
	if err := os.Rename(path, destination); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		source := path + suffix
		if _, err := os.Stat(source); err != nil {
			continue
		}
		if err := os.Rename(source, destination+suffix); err != nil {
			return err
		}
	}
	return nil
}

func archiveDestination(path string) (string, error) {
	destination := path + ".migrated"
	_, err := os.Stat(destination)
	if os.IsNotExist(err) {
		return destination, nil
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%d", destination, time.Now().UTC().Unix()), nil
}
