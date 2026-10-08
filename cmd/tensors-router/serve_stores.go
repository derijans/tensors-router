package main

import (
	"context"
	"log"
	"path/filepath"
	"strings"

	routeranalytics "tensors-router/internal/analytics"
	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/buildinfo"
	"tensors-router/internal/config"
	"tensors-router/internal/loadcapture"
	"tensors-router/internal/loaderrors"
	"tensors-router/internal/modelstate"
	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/offloadsettings"
	"tensors-router/internal/recipes"
	"tensors-router/internal/routerstore"
	"tensors-router/internal/routinggroups"
)

type serveStores struct {
	handle           *routerstore.Handle
	benchmarks       *routerbenchmark.Store
	modelState       *modelstate.Store
	routingGroups    *routinggroups.Store
	offloadDecisions *offloaddecisions.Store
	analytics        *routeranalytics.Store
	loadCaptures     *loadcapture.Store
	loadErrors       *loaderrors.Store
	recipes          *recipes.Store
}

func (stores *serveStores) open(ctx context.Context, cfg config.Config, logger *log.Logger) error {
	var err error
	stores.handle, err = routerstore.Open(ctx, routerstore.Config{
		Path:          cfg.Cluster.DatabasePath,
		BinaryVersion: buildinfo.Current().Version,
		Modules: []routerstore.Module{
			routeranalytics.SchemaModule{},
			loadcapture.SchemaModule{},
			loaderrors.SchemaModule{},
			routinggroups.SchemaModule{},
			offloaddecisions.SchemaModule{},
			offloadsettings.SchemaModule{},
		},
		LegacySources: legacySources(cfg),
		Logger:        logger,
	})
	if err != nil {
		return err
	}
	if stores.benchmarks, err = routerbenchmark.NewStore(cfg.Cluster.StoreDir); err != nil {
		return err
	}
	if stores.modelState, err = modelstate.NewStore(cfg.Cluster.StoreDir); err != nil {
		return err
	}
	stores.routingGroups = routinggroups.NewStore(stores.handle.DB(), stores.handle.Reader())
	stores.offloadDecisions, err = offloaddecisions.NewStore(offloaddecisions.StoreConfig{
		NodeID:        cfg.Cluster.NodeID,
		RouterVersion: buildinfo.Current().Version,
		DB:            stores.handle.DB(),
		ReadDB:        stores.handle.Reader(),
		Logger:        logger,
	})
	if err != nil {
		return err
	}
	if stores.analytics, err = newAnalyticsStore(cfg, stores.handle, logger); err != nil {
		return err
	}
	if stores.loadCaptures, err = newLoadCaptureStore(cfg, stores.handle, logger); err != nil {
		return err
	}
	if stores.loadErrors, err = newLoadErrorStore(cfg, stores.handle); err != nil {
		return err
	}
	stores.recipes, err = recipes.NewStore(cfg.Cluster.StoreDir)
	return err
}

func newAnalyticsStore(cfg config.Config, handle *routerstore.Handle, logger *log.Logger) (*routeranalytics.Store, error) {
	if !cfg.Analytics.Enabled {
		return nil, nil
	}
	return routeranalytics.NewStore(routeranalytics.StoreConfig{
		NodeID:        cfg.Cluster.NodeID,
		RouterVersion: buildinfo.Current().Version,
		DB:            handle.DB(),
		ReadDB:        handle.Reader(),
		FlushInterval: cfg.Analytics.FlushInterval,
		RawRetention:  cfg.Analytics.RawRetention,
		Logger:        logger,
	})
}

func newLoadCaptureStore(cfg config.Config, handle *routerstore.Handle, logger *log.Logger) (*loadcapture.Store, error) {
	if !cfg.Analytics.LoadCaptureEnabled {
		return nil, nil
	}
	return loadcapture.NewStore(loadcapture.StoreConfig{NodeID: cfg.Cluster.NodeID, DB: handle.DB(), ReadDB: handle.Reader(), Logger: logger})
}

func newLoadErrorStore(cfg config.Config, handle *routerstore.Handle) (*loaderrors.Store, error) {
	if !cfg.Diagnostics.Enabled {
		return nil, nil
	}
	return loaderrors.NewStore(loaderrors.StoreConfig{
		NodeID:         cfg.Cluster.NodeID,
		DB:             handle.DB(),
		ReadDB:         handle.Reader(),
		Retention:      cfg.Diagnostics.Retention,
		MaxOutputBytes: int(cfg.Diagnostics.MaxOutputKB) << 10,
	})
}

// A node that ran before the databases were joined still holds its rows in the
// file the old key named, and in the file the old default named, so both are
// offered to the importer.
func legacySources(cfg config.Config) []routerstore.LegacySource {
	candidates := []routerstore.LegacySource{
		{Module: "analytics", Path: cfg.Analytics.DatabasePath},
		{Module: "analytics", Path: filepath.Join(cfg.Cluster.StoreDir, "analytics.sqlite")},
		{Module: "loadcapture", Path: cfg.Analytics.LoadCaptureDatabasePath},
		{Module: "loadcapture", Path: filepath.Join(cfg.Cluster.StoreDir, "load-captures.sqlite")},
		{Module: "loaderrors", Path: cfg.Diagnostics.DatabasePath},
		{Module: "loaderrors", Path: filepath.Join(cfg.Cluster.StoreDir, "load-errors.sqlite")},
		{Module: "routinggroups", Path: filepath.Join(cfg.Cluster.StoreDir, "routing-groups.sqlite")},
	}
	sources := make([]routerstore.LegacySource, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Path) == "" {
			continue
		}
		sources = append(sources, candidate)
	}
	return sources
}
