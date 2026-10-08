package main

import (
	"context"

	routercluster "tensors-router/internal/cluster"
	"tensors-router/internal/config"
)

func (runtime *serveRuntime) connectCluster() {
	cfg := runtime.cfg
	runtime.clusterProbeClient = routercluster.NewClientWithTimeout(cfg.Cluster.Token, cfg.Cluster.ControlTimeout, clusterProbeTargets(cfg)...)
	runtime.clusterClient = routercluster.NewClientWithTimeout(cfg.Cluster.Token, cfg.Cluster.ControlTimeout, clusterRoutingTargets(cfg)...)
}

func (runtime *serveRuntime) clusterSyncConfig() routercluster.SyncConfig {
	cfg := runtime.cfg
	clusterClient := runtime.clusterClient
	return routercluster.SyncConfig{
		Role:            cfg.Cluster.Role,
		MasterURL:       cfg.Cluster.MasterURL,
		SlaveURLs:       cfg.Cluster.SlaveURLs,
		SyncInterval:    cfg.Cluster.SyncInterval,
		HealthInterval:  cfg.Cluster.HealthInterval,
		SyncConcurrency: cfg.Cluster.SyncConcurrency,
		AcceptNodeURL:   func(nodeURL string) error { return clusterClient.AllowBaseURLs(nodeURL) },
	}
}

func (runtime *serveRuntime) syncConfiguredSlaves(ctx context.Context, syncConfig routercluster.SyncConfig) {
	routercluster.SyncConfiguredSlaves(ctx, syncConfig, runtime.registry, runtime.clusterProbeClient, runtime.logger)
}

func clusterProbeTargets(cfg config.Config) []string {
	targets := []string{
		cfg.Cluster.PublicURL,
		cfg.Cluster.MasterURL,
	}
	targets = append(targets, cfg.Cluster.SlaveURLs...)
	return targets
}

func clusterRoutingTargets(cfg config.Config) []string {
	return []string{cfg.Cluster.PublicURL, cfg.Cluster.MasterURL}
}
