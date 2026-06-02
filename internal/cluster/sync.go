package cluster

import (
	"context"
	"log"
	"time"
)

type SyncConfig struct {
	Role           string
	MasterURL      string
	SlaveURLs      []string
	SyncInterval   time.Duration
	HealthInterval time.Duration
}

func SyncConfiguredSlaves(ctx context.Context, config SyncConfig, registry *Registry, client *Client, logger *log.Logger) {
	if config.Role != RoleMaster {
		return
	}
	for _, slaveURL := range config.SlaveURLs {
		syncSlave(ctx, registry, client, logger, slaveURL)
	}
}

func StartSync(ctx context.Context, config SyncConfig, registry *Registry, client *Client, logger *log.Logger) {
	switch config.Role {
	case RoleMaster:
		go syncSlavesLoop(ctx, config, registry, client, logger)
	case RoleSlave:
		go registerLoop(ctx, config, registry, client, logger)
	}
}

func syncSlavesLoop(ctx context.Context, config SyncConfig, registry *Registry, client *Client, logger *log.Logger) {
	ticker := time.NewTicker(config.HealthInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, slaveURL := range config.SlaveURLs {
				syncSlave(ctx, registry, client, logger, slaveURL)
			}
		}
	}
}

func registerLoop(ctx context.Context, config SyncConfig, registry *Registry, client *Client, logger *log.Logger) {
	registerWithMaster(ctx, registry, client, logger, config.MasterURL)
	ticker := time.NewTicker(config.SyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			registerWithMaster(ctx, registry, client, logger, config.MasterURL)
		}
	}
}

func syncSlave(ctx context.Context, registry *Registry, client *Client, logger *log.Logger, slaveURL string) {
	snapshot, err := client.FetchSnapshot(ctx, slaveURL)
	if err != nil {
		registry.MarkNodeURLHealth(slaveURL, false)
		logger.Printf("cluster slave sync failed url=%s error=%v", slaveURL, err)
		return
	}
	if snapshot.NodeURL == "" {
		snapshot.NodeURL = slaveURL
	}
	if err := registry.UpdateNode(snapshot); err != nil {
		logger.Printf("cluster slave update failed url=%s error=%v", slaveURL, err)
	}
}

func registerWithMaster(ctx context.Context, registry *Registry, client *Client, logger *log.Logger, masterURL string) {
	if err := client.Register(ctx, masterURL, registry.Snapshot()); err != nil {
		logger.Printf("cluster master registration failed url=%s error=%v", masterURL, err)
	}
}
