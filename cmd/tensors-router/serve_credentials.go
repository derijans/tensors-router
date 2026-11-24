package main

import (
	"context"
	"net/url"
	"os"
	"path/filepath"

	"tensors-router/internal/credentialstore"
	"tensors-router/internal/routertls"
)

type serveCredentials struct {
	store       *credentialstore.Store
	live        *credentialstore.Live
	tls         *routertls.Runtime
	peers       *routertls.PeerTransport
	listenerTLS bool
	warnings    []string
	stopWatch   context.CancelFunc
}

func (runtime *serveRuntime) openCredentials(ctx context.Context) error {
	cfg := runtime.cfg
	store, err := openCredentialStore(ctx, cfg, runtime.logger)
	if err != nil {
		return err
	}
	live, err := credentialstore.NewLive(ctx, store, runtime.logger)
	if err != nil {
		_ = store.Close()
		return err
	}
	coverageWarnings, err := cfg.CheckStoredCredentials(storedCredentialCoverage(live.Current()))
	if err != nil {
		_ = store.Close()
		return err
	}
	tlsRuntime := routertls.NewRuntime(live)
	watchContext, stopWatch := context.WithCancel(ctx)
	go live.Watch(watchContext, credentialstore.DefaultWatchInterval)
	runtime.credentials = serveCredentials{
		stopWatch:   stopWatch,
		store:       store,
		live:        live,
		tls:         tlsRuntime,
		peers:       tlsRuntime.PeerTransport(),
		listenerTLS: tlsRuntime.Enabled(),
		warnings:    append(ignoredTokenSettingsWarning(cfg), coverageWarnings...),
	}
	runtime.credentials.warnings = append(runtime.credentials.warnings, plainPeerURLWarnings(runtime.credentials.listenerTLS, clusterProbeTargets(cfg))...)
	for _, warning := range runtime.credentials.warnings {
		runtime.startupLogger.Printf("configuration warning: %s", warning)
	}
	return nil
}

func plainPeerURLWarnings(listenerTLS bool, peerURLs []string) []string {
	if !listenerTLS {
		return nil
	}
	var warnings []string
	for _, peerURL := range peerURLs {
		if parsed, err := url.Parse(peerURL); err == nil && parsed.Scheme == "http" {
			warnings = append(warnings, "cluster peer "+peerURL+" uses http while this router serves HTTPS; use https so node traffic is encrypted")
		}
	}
	return warnings
}

func (runtime *serveRuntime) exportTrustBundle(ctx context.Context, path string) {
	if path == "" {
		return
	}
	write := func(snapshot *credentialstore.Snapshot) {
		if err := writeTrustBundle(ctx, runtime.credentials.store, path, snapshot.TLSEnabled()); err != nil {
			runtime.logger.Printf("trust bundle export failed: %v", err)
		}
	}
	write(runtime.credentials.live.Current())
	runtime.credentials.live.OnChange(write)
}

func writeTrustBundle(ctx context.Context, store *credentialstore.Store, path string, tlsEnabled bool) error {
	if !tlsEnabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	bundle, err := store.TrustBundle(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, bundle, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func (credentials serveCredentials) close() error {
	if credentials.stopWatch != nil {
		credentials.stopWatch()
	}
	if credentials.peers != nil {
		credentials.peers.CloseIdleConnections()
	}
	return credentials.store.Close()
}
