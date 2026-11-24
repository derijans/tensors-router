package credentialstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"tensors-router/internal/credential"
)

func (store *Store) EnsureClusterToken(ctx context.Context, name string, secret string) (Token, bool, error) {
	baseName, err := normalizedName(name)
	if err != nil {
		return Token{}, false, err
	}
	secret, source, err := tokenSecret(secret)
	if err != nil {
		return Token{}, false, err
	}
	if source != SourceSupplied {
		return Token{}, false, inputError("cluster token value is required")
	}
	var token Token
	created := false
	err = store.write(ctx, func(tx *sql.Tx) error {
		existing, found, err := tokenWithSecret(ctx, tx, secret)
		if err != nil || found {
			token = existing
			return err
		}
		freeName, err := freeTokenName(ctx, tx, baseName)
		if err != nil {
			return err
		}
		token = Token{ID: newID(), Name: freeName, Role: RoleCluster, Source: SourceSupplied, CreatedAt: store.now().UTC()}
		created = true
		return insertToken(ctx, tx, token, secret)
	})
	if err != nil {
		return Token{}, false, err
	}
	if !created && token.Role != RoleCluster {
		return Token{}, false, inputError("the cluster token value is already stored as a %s token on this router", token.Role)
	}
	return token, created, nil
}

func tokenWithSecret(ctx context.Context, tx *sql.Tx, secret string) (Token, bool, error) {
	hash := credential.HashToken(secret)
	var token Token
	var createdAt int64
	err := tx.QueryRowContext(ctx, `SELECT id, name, role, source, created_at FROM credential_tokens WHERE secret_hash = ?`, hash[:]).
		Scan(&token.ID, &token.Name, &token.Role, &token.Source, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Token{}, false, nil
	}
	if err != nil {
		return Token{}, false, err
	}
	token.CreatedAt = time.Unix(createdAt, 0).UTC()
	return token, true, nil
}

func freeTokenName(ctx context.Context, tx *sql.Tx, baseName string) (string, error) {
	name := baseName
	for suffix := 2; ; suffix++ {
		taken, err := rowExists(ctx, tx, `SELECT 1 FROM credential_tokens WHERE name = ?`, name)
		if err != nil || !taken {
			return name, err
		}
		name = fmt.Sprintf("%s-%d", baseName, suffix)
	}
}
