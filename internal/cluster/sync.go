package cluster

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

type SyncConfig struct {
	Role            string
	MasterURL       string
	SlaveURLs       []string
	SyncInterval    time.Duration
	HealthInterval  time.Duration
	SyncConcurrency int
	AcceptNodeURL   func(string) error
}

type slaveSyncResult struct {
	url      string
	snapshot Snapshot
	err      error
}

func SyncConfiguredSlaves(ctx context.Context, config SyncConfig, registry *Registry, client *Client, logger *log.Logger) {
	if config.Role != RoleMaster {
		return
	}
	syncSlaves(ctx, config, registry, client, logger)
}

func RegisterInitial(ctx context.Context, config SyncConfig, registry *Registry, client *Client, logger *log.Logger) error {
	if config.Role != RoleSlave {
		return nil
	}
	response, err := client.Register(ctx, config.MasterURL, registry.Snapshot())
	if err == nil {
		return checkMasterCompatibility(response, config.MasterURL, logger)
	}
	if terminalRegistrationError(err) {
		return err
	}
	logSyncError(logger, "cluster initial master registration failed url=%s error=%v", config.MasterURL, err)
	return nil
}

func checkMasterCompatibility(response RegisterResponse, masterURL string, logger *log.Logger) error {
	if response.ProtocolVersion >= MinimumProtocolVersion {
		return nil
	}
	err := fmt.Errorf("master %q speaks protocol version %d, below the minimum %d this build requires; update the master", masterURL, response.ProtocolVersion, MinimumProtocolVersion)
	logSyncError(logger, "cluster registration stopped, incompatible master: %v", err)
	return err
}

func StartSync(ctx context.Context, config SyncConfig, registry *Registry, client *Client, logger *log.Logger) <-chan error {
	errCh := make(chan error, 1)
	switch config.Role {
	case RoleMaster:
		go syncSlavesLoop(ctx, config, registry, client, logger)
	case RoleSlave:
		go registerLoop(ctx, config, registry, client, logger, errCh)
	}
	return errCh
}

func syncSlavesLoop(ctx context.Context, config SyncConfig, registry *Registry, client *Client, logger *log.Logger) {
	ticker := time.NewTicker(config.HealthInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncSlaves(ctx, config, registry, client, logger)
		}
	}
}

func registerLoop(ctx context.Context, config SyncConfig, registry *Registry, client *Client, logger *log.Logger, errCh chan<- error) {
	ticker := time.NewTicker(config.SyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			response, err := client.Register(ctx, config.MasterURL, registry.Snapshot())
			if err == nil {
				if compatErr := checkMasterCompatibility(response, config.MasterURL, logger); compatErr != nil {
					select {
					case errCh <- compatErr:
					default:
					}
					return
				}
				continue
			}
			if terminalRegistrationError(err) {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			logSyncError(logger, "cluster master registration failed url=%s error=%v", config.MasterURL, err)
		}
	}
}

func terminalRegistrationError(err error) bool {
	var remoteError *RemoteError
	return errors.As(err, &remoteError) &&
		remoteError.StatusCode == http.StatusConflict &&
		remoteError.Type == "cluster_error" &&
		remoteError.Code == ErrorCodeDuplicateNode
}

func syncSlaves(ctx context.Context, config SyncConfig, registry *Registry, client *Client, logger *log.Logger) {
	results := fetchSlaveSnapshots(ctx, config.SlaveURLs, config.SyncConcurrency, client)
	for _, result := range results {
		if result.err != nil {
			registry.MarkNodeURLHealth(result.url, false)
			logSyncError(logger, "cluster slave sync failed url=%s error=%v", result.url, result.err)
			continue
		}
		result.snapshot.NodeURL = result.url
		if err := registry.UpdateNode(result.snapshot); err != nil {
			logSyncError(logger, "cluster slave update failed url=%s error=%v", result.url, err)
			continue
		}
		if config.AcceptNodeURL != nil {
			if err := config.AcceptNodeURL(result.url); err != nil {
				registry.MarkNodeURLHealth(result.url, false)
				logSyncError(logger, "cluster slave authorization failed url=%s error=%v", result.url, err)
			}
		}
	}
}

func fetchSlaveSnapshots(ctx context.Context, urls []string, concurrency int, client *Client) []slaveSyncResult {
	if concurrency <= 0 {
		concurrency = 4
	}
	if concurrency > len(urls) && len(urls) > 0 {
		concurrency = len(urls)
	}
	results := make([]slaveSyncResult, len(urls))
	semaphore := make(chan struct{}, concurrency)
	var waitGroup sync.WaitGroup
	for index, slaveURL := range urls {
		index := index
		slaveURL := slaveURL
		results[index].url = slaveURL
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results[index].err = ctx.Err()
				return
			}
			results[index].snapshot, results[index].err = client.FetchSnapshot(ctx, slaveURL)
		}()
	}
	waitGroup.Wait()
	return results
}

func logSyncError(logger *log.Logger, format string, values ...any) {
	if logger != nil {
		logger.Printf(format, values...)
	}
}
