package credentialstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"tensors-router/internal/credential"
)

const (
	generatedTokenPrefix  = "trt_"
	generatedTokenEntropy = 32
)

type NewToken struct {
	Name     string
	Role     Role
	Supplied string
}

func (store *Store) CreateToken(ctx context.Context, request NewToken) (Token, string, error) {
	name, err := normalizedName(request.Name)
	if err != nil {
		return Token{}, "", err
	}
	if _, err := ParseRole(string(request.Role)); err != nil {
		return Token{}, "", err
	}
	secret, source, err := tokenSecret(request.Supplied)
	if err != nil {
		return Token{}, "", err
	}
	token := Token{ID: newID(), Name: name, Role: request.Role, Source: source, CreatedAt: store.now().UTC()}
	err = store.write(ctx, func(tx *sql.Tx) error {
		return insertToken(ctx, tx, token, secret)
	})
	if err != nil {
		return Token{}, "", err
	}
	return token, secret, nil
}

func tokenSecret(supplied string) (string, string, error) {
	supplied = strings.TrimSpace(supplied)
	if supplied == "" {
		secret, err := generateTokenSecret()
		return secret, SourceCreated, err
	}
	if len(supplied) < credential.RecommendedMinimumLength {
		return "", "", inputError("token value must be at least %d characters", credential.RecommendedMinimumLength)
	}
	if strings.ContainsAny(supplied, " \t\r\n") {
		return "", "", inputError("token value must not contain whitespace")
	}
	if err := credential.RejectPlaceholder("token value", supplied); err != nil {
		return "", "", InputError{message: err.Error()}
	}
	return supplied, SourceSupplied, nil
}

func generateTokenSecret() (string, error) {
	entropy := make([]byte, generatedTokenEntropy)
	if _, err := rand.Read(entropy); err != nil {
		return "", err
	}
	return generatedTokenPrefix + base64.RawURLEncoding.EncodeToString(entropy), nil
}

func insertToken(ctx context.Context, tx *sql.Tx, token Token, secret string) error {
	hash := credential.HashToken(secret)
	conflict, err := rowExists(ctx, tx, `SELECT 1 FROM credential_tokens WHERE name = ? OR secret_hash = ?`, token.Name, hash[:])
	if err != nil {
		return err
	}
	if conflict {
		return ErrConflict
	}
	var clusterSecret any
	if token.Role == RoleCluster {
		clusterSecret = secret
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO credential_tokens (id, name, role, secret_hash, cluster_secret, source, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		token.ID, token.Name, string(token.Role), hash[:], clusterSecret, token.Source, token.CreatedAt.Unix())
	return err
}

func (store *Store) Tokens(ctx context.Context) ([]Token, error) {
	rows, err := store.handle.Reader().QueryContext(ctx, `SELECT id, name, role, source, created_at
		FROM credential_tokens ORDER BY role, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tokens := []Token{}
	for rows.Next() {
		var token Token
		var createdAt int64
		if err := rows.Scan(&token.ID, &token.Name, &token.Role, &token.Source, &createdAt); err != nil {
			return nil, err
		}
		token.CreatedAt = time.Unix(createdAt, 0).UTC()
		tokens = append(tokens, token)
	}
	return tokens, rows.Err()
}

func (store *Store) DeleteToken(ctx context.Context, reference string) error {
	return store.write(ctx, func(tx *sql.Tx) error {
		id, role, err := resolveToken(ctx, tx, reference)
		if err != nil {
			return err
		}
		if err := store.ensureNotLastProtected(ctx, tx, role); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM credential_tokens WHERE id = ?`, id)
		return err
	})
}

func (store *Store) ensureNotLastProtected(ctx context.Context, tx *sql.Tx, role Role) error {
	if !store.protectedRoles[role] {
		return nil
	}
	var remaining int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM credential_tokens WHERE role = ?`, string(role)).Scan(&remaining); err != nil {
		return err
	}
	if remaining <= 1 {
		return inputError("refusing to delete the last %s token: this router configuration requires one; create a replacement first", role)
	}
	return nil
}

func (store *Store) ClusterSecret(ctx context.Context, reference string) (string, error) {
	var secret sql.NullString
	err := store.handle.Reader().QueryRowContext(ctx, `SELECT cluster_secret FROM credential_tokens WHERE id = ? OR name = ?`,
		strings.TrimSpace(reference), strings.TrimSpace(reference)).Scan(&secret)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if !secret.Valid {
		return "", inputError("only cluster tokens can be shown again; other tokens are stored as hashes")
	}
	return secret.String, nil
}

func resolveToken(ctx context.Context, tx *sql.Tx, reference string) (string, Role, error) {
	reference = strings.TrimSpace(reference)
	var id string
	var role Role
	err := tx.QueryRowContext(ctx, `SELECT id, role FROM credential_tokens WHERE id = ? OR name = ?`, reference, reference).Scan(&id, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return id, role, err
}
