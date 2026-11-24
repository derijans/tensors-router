package main

import (
	"slices"
	"testing"

	"tensors-router/internal/config"
)

func TestDefaultIdentityHostsListEachHostOnce(t *testing.T) {
	var cfg config.Config
	cfg.Server.Bind = "127.0.0.1:18290"
	cfg.Cluster.PublicURL = "https://127.0.0.1:18291"
	hosts := defaultIdentityHosts(cfg)
	if count := len(slices.DeleteFunc(slices.Clone(hosts), func(host string) bool { return host != "127.0.0.1" })); count != 1 {
		t.Fatalf("default identity hosts %v list 127.0.0.1 %d times; want once", hosts, count)
	}
}
