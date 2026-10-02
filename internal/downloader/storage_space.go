package downloader

import (
	"fmt"
	"sync"
)

func (manager *Manager) checkFreeSpace(required int64) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.checkFreeSpaceLocked(required)
}

func (manager *Manager) reserveSpace(required int64) (func(), error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.checkFreeSpaceLocked(required); err != nil {
		return nil, err
	}
	manager.reservedBytes += required
	var release sync.Once
	return func() {
		release.Do(func() {
			manager.mu.Lock()
			manager.reservedBytes -= required
			manager.mu.Unlock()
		})
	}, nil
}

func (manager *Manager) checkFreeSpaceLocked(required int64) error {
	reserve := manager.config.Storage.FreeSpaceReserveGB << 30
	for _, directory := range []string{manager.config.Storage.Root, manager.config.Storage.StateDir} {
		if directory == "" {
			continue
		}
		available, known, err := availableSpace(directory)
		if err != nil {
			return err
		}
		if !known {
			continue
		}
		if available-manager.reservedBytes-required < reserve {
			return fmt.Errorf("insufficient storage space in %s: %s free, %s needed by this download, %s held by other downloads, %s reserve", directory, formatBytes(available), formatBytes(required), formatBytes(manager.reservedBytes), formatBytes(reserve))
		}
	}
	return nil
}

func formatBytes(value int64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor, exponent := int64(unit), 0
	for quotient := value / unit; quotient >= unit && exponent < 4; quotient /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(divisor), "KMGTP"[exponent])
}
