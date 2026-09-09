package routerstore

import "os"

func secureDatabaseFiles(path string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		candidate := path + suffix
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := os.Chmod(candidate, 0o600); err != nil {
			return err
		}
	}
	return nil
}
