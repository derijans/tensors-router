package downloader

import "context"

func (manager *Manager) ScanKnownArtifacts(ctx context.Context) error {
	manager.scanMu.Lock()
	defer manager.scanMu.Unlock()
	_, unknown, err := manager.scanLibrary(ctx)
	if err != nil {
		return err
	}
	manager.library.replace(unknown)
	return nil
}

func (manager *Manager) startKnownArtifactScan() {
	manager.scans.Add(1)
	go func() {
		defer manager.scans.Done()
		if err := manager.ScanKnownArtifacts(manager.lifecycle); err != nil && manager.lifecycle.Err() == nil {
			manager.logRuntime("download library start scan failed error=%q", err)
		}
	}()
}
