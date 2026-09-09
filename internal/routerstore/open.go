package routerstore

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const (
	defaultFilename = "analytics.sqlite"
	readerPoolSize  = 4
)

type Config struct {
	Path          string
	Modules       []Module
	LegacySources []LegacySource
	Logger        *log.Logger
}

type Handle struct {
	path     string
	writer   *sql.DB
	reader   *sql.DB
	logger   *log.Logger
	warnings []string
}

func DefaultPath(storeDir string) string {
	return filepath.Join(storeDir, defaultFilename)
}

func Open(ctx context.Context, config Config) (*Handle, error) {
	path := strings.TrimSpace(config.Path)
	if path == "" {
		return nil, fmt.Errorf("router database path is required")
	}
	if !pathAcceptedByDriver(path) {
		return nil, fmt.Errorf("router database path must not contain a question mark: %s", path)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, err
	}
	writer, err := sql.Open("sqlite", writerDSN(path))
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	writer.SetConnMaxLifetime(0)
	writer.SetConnMaxIdleTime(0)
	logger := config.Logger
	if logger == nil {
		logger = log.Default()
	}
	handle := &Handle{path: path, writer: writer, logger: logger}
	if err := prepareDatabase(ctx, handle, config.Modules); err != nil {
		_ = writer.Close()
		return nil, err
	}
	handle.importLegacySources(ctx, config.Modules, config.LegacySources)
	reader, err := sql.Open("sqlite", readerDSN(path))
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	reader.SetMaxOpenConns(readerPoolSize)
	reader.SetMaxIdleConns(readerPoolSize)
	handle.reader = reader
	if err := secureDatabaseFiles(path); err != nil {
		_ = handle.Close()
		return nil, err
	}
	return handle, nil
}

func prepareDatabase(ctx context.Context, handle *Handle, modules []Module) error {
	if err := applyFileFormatVersion(ctx, handle.writer); err != nil {
		return err
	}
	if err := createRegistryTables(ctx, handle.writer); err != nil {
		return err
	}
	return migrateModules(ctx, handle.writer, modules)
}

func (handle *Handle) DB() *sql.DB {
	if handle == nil {
		return nil
	}
	return handle.writer
}

func (handle *Handle) Reader() *sql.DB {
	if handle == nil {
		return nil
	}
	return handle.reader
}

func (handle *Handle) Path() string {
	if handle == nil {
		return ""
	}
	return handle.path
}

func (handle *Handle) Warnings() []string {
	if handle == nil {
		return nil
	}
	return append([]string(nil), handle.warnings...)
}

func (handle *Handle) Close() error {
	if handle == nil {
		return nil
	}
	var closeErr error
	if handle.reader != nil {
		closeErr = handle.reader.Close()
		handle.reader = nil
	}
	if handle.writer != nil {
		if err := handle.writer.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		handle.writer = nil
	}
	return closeErr
}

func (handle *Handle) warn(format string, arguments ...any) {
	message := fmt.Sprintf(format, arguments...)
	handle.warnings = append(handle.warnings, message)
	handle.logger.Print(message)
}
