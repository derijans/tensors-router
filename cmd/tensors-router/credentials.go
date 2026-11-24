package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"

	"tensors-router/internal/buildinfo"
	"tensors-router/internal/certgen"
	"tensors-router/internal/config"
	"tensors-router/internal/credentialstore"
)

func openCredentialStore(ctx context.Context, cfg config.Config, logger *log.Logger) (*credentialstore.Store, error) {
	store, err := credentialstore.Open(ctx, credentialstore.Config{
		Path:           credentialstore.DefaultPath(cfg.Cluster.StoreDir),
		BinaryVersion:  buildinfo.Current().Version,
		ProtectedRoles: protectedTokenRoles(cfg),
		Logger:         logger,
	})
	if err != nil {
		return nil, err
	}
	imported, err := store.ImportConfiguredOnce(ctx, configuredCredentials(cfg))
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("import configured tokens: %w", err)
	}
	if imported > 0 {
		logger.Printf("imported %d configured tokens into %s", imported, store.Path())
	}
	return store, nil
}

func protectedTokenRoles(cfg config.Config) []credentialstore.Role {
	inference, admin, cluster := cfg.RequiresProtectedTokens()
	var roles []credentialstore.Role
	if inference {
		roles = append(roles, credentialstore.RoleInference)
	}
	if admin {
		roles = append(roles, credentialstore.RoleAdmin)
	}
	if cluster {
		roles = append(roles, credentialstore.RoleCluster)
	}
	return roles
}

func configuredCredentials(cfg config.Config) credentialstore.ConfiguredCredentials {
	return credentialstore.ConfiguredCredentials{
		InferenceKeys: cfg.Auth.InferenceKeys,
		AdminKeys:     cfg.Auth.AdminKeys,
		MetricsKeys:   cfg.Auth.MetricsKeys,
		ClusterToken:  cfg.Cluster.Token,
	}
}

func ignoredTokenSettingsWarning(cfg config.Config) []string {
	settings := cfg.ConfiguredTokenSettings()
	if len(settings) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("%s and the TENSORS_ROUTER_*_TOKEN environment variables are ignored after the one-time import into %s; manage tokens with `tensors-router security token` or the WebUI Security tab and remove them from configuration",
		strings.Join(settings, ", "), credentialstore.Filename)}
}

func storedCredentialCoverage(snapshot *credentialstore.Snapshot) config.StoredCredentials {
	return config.StoredCredentials{
		Inference: len(snapshot.Keys.Inference) > 0,
		Admin:     len(snapshot.Keys.Admin) > 0,
		Cluster:   len(snapshot.Keys.Cluster) > 0,
	}
}

func defaultIdentityHosts(cfg config.Config) []string {
	hosts := certgen.LoopbackHosts()
	if bindHost := certgen.BindHost(cfg.Server.Bind); certgen.IsWildcardHost(bindHost) {
		hosts = append(hosts, certgen.LocalInterfaceHosts()...)
	} else {
		hosts = append(hosts, bindHost)
	}
	if publicURL, err := url.Parse(cfg.Cluster.PublicURL); err == nil && publicURL.Hostname() != "" {
		hosts = append(hosts, publicURL.Hostname())
	}
	return hosts
}
