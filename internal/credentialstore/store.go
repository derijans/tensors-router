package credentialstore

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"tensors-router/internal/routerstore"
)

const Filename = "credentials.sqlite"

type Config struct {
	Path           string
	BinaryVersion  string
	ProtectedRoles []Role
	Logger         *log.Logger
}

type Store struct {
	handle         *routerstore.Handle
	protectedRoles map[Role]bool
	now            func() time.Time
}

func DefaultPath(storeDir string) string {
	return filepath.Join(storeDir, Filename)
}

func Open(ctx context.Context, config Config) (*Store, error) {
	handle, err := routerstore.Open(ctx, routerstore.Config{
		Path:          config.Path,
		BinaryVersion: config.BinaryVersion,
		Modules:       []routerstore.Module{SchemaModule{}},
		Logger:        config.Logger,
	})
	if err != nil {
		return nil, err
	}
	protected := map[Role]bool{}
	for _, role := range config.ProtectedRoles {
		protected[role] = true
	}
	return &Store{handle: handle, protectedRoles: protected, now: time.Now}, nil
}

func (store *Store) Close() error {
	if store == nil {
		return nil
	}
	return store.handle.Close()
}

func (store *Store) Path() string {
	return store.handle.Path()
}

func (store *Store) Revision(ctx context.Context) (int64, error) {
	var revision int64
	err := store.handle.Reader().QueryRowContext(ctx, `SELECT revision FROM credential_revision WHERE id = 1`).Scan(&revision)
	return revision, err
}

func (store *Store) write(ctx context.Context, change func(tx *sql.Tx) error) error {
	tx, err := store.handle.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := change(tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE credential_revision SET revision = revision + 1 WHERE id = 1`); err != nil {
		return err
	}
	return tx.Commit()
}

func newID() string {
	return uuid.NewString()
}

func rowExists(ctx context.Context, tx *sql.Tx, query string, arguments ...any) (bool, error) {
	var found int
	err := tx.QueryRowContext(ctx, query, arguments...).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func requireAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
