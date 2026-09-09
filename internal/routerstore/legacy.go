package routerstore

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const legacySchemaName = "legacy_import"

type LegacySource struct {
	Module string
	Path   string
}

func (handle *Handle) importLegacySources(ctx context.Context, modules []Module, sources []LegacySource) {
	byName := make(map[string]Module, len(modules))
	for _, module := range modules {
		byName[module.Name()] = module
	}
	attempted := make(map[string]bool, len(sources))
	for _, source := range sources {
		module, known := byName[source.Module]
		if !known {
			continue
		}
		path := strings.TrimSpace(source.Path)
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			handle.warn("legacy database path could not be resolved module=%s source=%s error=%v", source.Module, path, err)
			continue
		}
		key := source.Module + "\x00" + absolute
		if attempted[key] {
			continue
		}
		attempted[key] = true
		if err := handle.importLegacySource(ctx, module, absolute); err != nil {
			handle.warn("legacy database import failed module=%s source=%s error=%v", module.Name(), absolute, err)
		}
	}
}

func (handle *Handle) importLegacySource(ctx context.Context, module Module, path string) error {
	legacyInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	mergedInfo, err := os.Stat(handle.path)
	if err == nil && os.SameFile(legacyInfo, mergedInfo) {
		return nil
	}
	recorded, err := legacyImportRecorded(ctx, handle.writer, module.Name(), path)
	if err != nil {
		return err
	}
	if recorded {
		handle.warn("legacy database %s was already imported but is still present; remove it manually", path)
		return nil
	}
	copied, err := handle.copyLegacySource(ctx, module, path)
	if err != nil {
		return err
	}
	handle.logger.Printf("legacy database imported module=%s source=%s rows=%d", module.Name(), path, copied)
	if err := archiveLegacyFile(path); err != nil {
		handle.warn("legacy database %s was imported but could not be archived; remove it manually: %v", path, err)
	}
	return nil
}

func (handle *Handle) copyLegacySource(ctx context.Context, module Module, path string) (int64, error) {
	conn, err := handle.writer.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS `+legacySchemaName, path); err != nil {
		return 0, err
	}
	defer func() {
		_, _ = conn.ExecContext(context.WithoutCancel(ctx), `DETACH DATABASE `+legacySchemaName)
	}()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return 0, err
	}
	copied, err := module.ImportLegacy(ctx, tx, legacySchemaName)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO routerstore_legacy_imports (module, source, rows_copied, imported_at) VALUES (?, ?, ?, ?)`,
		module.Name(), path, copied, time.Now().UTC().UnixMilli()); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return copied, nil
}

func legacyImportRecorded(ctx context.Context, db *sql.DB, module string, path string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM routerstore_legacy_imports WHERE module = ? AND source = ?`, module, path).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
