package credentialstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"tensors-router/internal/credential"
)

const configuredCredentialsImport = "router-config"

type ConfiguredCredentials struct {
	InferenceKeys []string
	AdminKeys     []string
	MetricsKeys   []string
	ClusterToken  string
}

func (configured ConfiguredCredentials) Present() bool {
	return len(configured.entries()) > 0
}

type configuredEntry struct {
	role   Role
	secret string
}

func (configured ConfiguredCredentials) entries() []configuredEntry {
	var entries []configuredEntry
	appendRole := func(role Role, values []string) {
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				entries = append(entries, configuredEntry{role: role, secret: value})
			}
		}
	}
	appendRole(RoleInference, configured.InferenceKeys)
	appendRole(RoleAdmin, configured.AdminKeys)
	appendRole(RoleMetrics, configured.MetricsKeys)
	appendRole(RoleCluster, []string{configured.ClusterToken})
	return entries
}

func (store *Store) ImportConfiguredOnce(ctx context.Context, configured ConfiguredCredentials) (int, error) {
	imported := 0
	err := store.write(ctx, func(tx *sql.Tx) error {
		done, err := rowExists(ctx, tx, `SELECT 1 FROM credential_imports WHERE source = ?`, configuredCredentialsImport)
		if err != nil || done {
			return err
		}
		perRole := map[Role]int{}
		for _, entry := range configured.entries() {
			added, err := store.importConfiguredEntry(ctx, tx, entry, perRole)
			if err != nil {
				return err
			}
			imported += added
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO credential_imports (source, imported_at) VALUES (?, ?)`, configuredCredentialsImport, store.now().Unix())
		return err
	})
	return imported, err
}

func (store *Store) importConfiguredEntry(ctx context.Context, tx *sql.Tx, entry configuredEntry, perRole map[Role]int) (int, error) {
	hash := credential.HashToken(entry.secret)
	duplicate, err := rowExists(ctx, tx, `SELECT 1 FROM credential_tokens WHERE secret_hash = ?`, hash[:])
	if err != nil || duplicate {
		return 0, err
	}
	perRole[entry.role]++
	token := Token{
		ID:        newID(),
		Name:      fmt.Sprintf("migrated-%s-%d", entry.role, perRole[entry.role]),
		Role:      entry.role,
		Source:    SourceMigrated,
		CreatedAt: store.now().UTC(),
	}
	if err := insertToken(ctx, tx, token, entry.secret); err != nil {
		return 0, err
	}
	return 1, nil
}
