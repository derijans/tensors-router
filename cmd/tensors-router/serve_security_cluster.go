package main

import (
	"tensors-router/internal/config"
	"tensors-router/internal/proxy"
)

func securityClusterSettings(cfg config.Config) proxy.SecurityClusterSettings {
	return proxy.SecurityClusterSettings{
		Role:       cfg.Cluster.Role,
		ServerBind: cfg.Server.Bind,
		Bind:       cfg.Cluster.Bind,
		PublicURL:  cfg.Cluster.PublicURL,
		MasterURL:  cfg.Cluster.MasterURL,
		SlaveURLs:  cfg.Cluster.SlaveURLs,
	}
}
