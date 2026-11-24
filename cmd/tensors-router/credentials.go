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

func openCredentialStore(ctx context.Context, cfg config.Config, logger *log.Logger, migratedAPICertificates ...credentialstore.APICertificateFiles) (*credentialstore.Store, error) {
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
	for _, files := range append([]credentialstore.APICertificateFiles{configuredAPICertificateFiles(cfg)}, migratedAPICertificates...) {
		imported, err := store.ImportAPICertificateFilesOnce(ctx, files)
		if err != nil {
			logger.Printf("configuration warning: %s API certificate not imported, the router retries at next start: %v", files.Source, err)
			continue
		}
		if imported {
			logger.Printf("imported %s API certificate %s into %s", files.Source, files.CertFile, store.Path())
		}
	}
	return store, nil
}

func configuredAPICertificateFiles(cfg config.Config) credentialstore.APICertificateFiles {
	return credentialstore.APICertificateFiles{Source: "router-config", CertFile: cfg.Server.TLSCertFile, KeyFile: cfg.Server.TLSKeyFile}
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

func ignoredCredentialSettingsWarnings(cfg config.Config) []string {
	var warnings []string
	if configuredAPICertificateFiles(cfg).Present() {
		warnings = append(warnings, fmt.Sprintf("server.tls_cert_file and server.tls_key_file are ignored after the one-time import into %s; manage the API certificate in the WebUI Security tab and remove them from configuration", credentialstore.Filename))
	}
	return append(warnings, ignoredTokenSettingsWarning(cfg)...)
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
	return uniqueHosts(hosts)
}

func uniqueHosts(hosts []string) []string {
	seen := map[string]bool{}
	unique := make([]string, 0, len(hosts))
	for _, host := range hosts {
		key := strings.ToLower(host)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, host)
		}
	}
	return unique
}
