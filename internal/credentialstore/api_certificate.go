package credentialstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"tensors-router/internal/certgen"
)

type APICertificateFiles struct {
	Source   string
	CertFile string
	KeyFile  string
}

func (files APICertificateFiles) Present() bool {
	return strings.TrimSpace(files.CertFile) != "" && strings.TrimSpace(files.KeyFile) != ""
}

func (files APICertificateFiles) importSource() string {
	return "api-certificate:" + files.Source
}

func (store *Store) ImportAPICertificateFilesOnce(ctx context.Context, files APICertificateFiles) (bool, error) {
	if !files.Present() {
		return false, nil
	}
	alreadyImported, err := store.importRecorded(ctx, files.importSource())
	if err != nil || alreadyImported {
		return false, err
	}
	request, err := files.read()
	if err != nil {
		return false, err
	}
	validated, err := validateImport(request, store.now())
	if err != nil {
		return false, fmt.Errorf("%s API certificate %s: %w", files.Source, files.CertFile, err)
	}
	imported := false
	err = store.write(ctx, func(tx *sql.Tx) error {
		known, err := rowExists(ctx, tx, `SELECT 1 FROM credential_certificates WHERE fingerprint = ?`, certgen.Fingerprint(validated.chain[0]))
		if err != nil {
			return err
		}
		if !known {
			if err := store.insertMigratedAPICertificate(ctx, tx, validated); err != nil {
				return err
			}
			imported = true
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO credential_imports (source, imported_at) VALUES (?, ?)`, files.importSource(), store.now().Unix())
		return err
	})
	return imported, err
}

func (files APICertificateFiles) read() (CertificateImport, error) {
	certificatePEM, err := os.ReadFile(files.CertFile)
	if err != nil {
		return CertificateImport{}, err
	}
	privateKeyPEM, err := os.ReadFile(files.KeyFile)
	if err != nil {
		return CertificateImport{}, err
	}
	return CertificateImport{
		Name:           "migrated-" + files.Source,
		Kind:           KindAPICertificate,
		CertificatePEM: string(certificatePEM),
		PrivateKeyPEM:  string(privateKeyPEM),
	}, nil
}

func (store *Store) insertMigratedAPICertificate(ctx context.Context, tx *sql.Tx, validated validatedCertificate) error {
	activeExists, err := rowExists(ctx, tx, `SELECT 1 FROM credential_certificates WHERE kind = ? AND active = 1`, string(KindAPICertificate))
	if err != nil {
		return err
	}
	name := validated.name
	for suffix := 2; ; suffix++ {
		taken, err := rowExists(ctx, tx, `SELECT 1 FROM credential_certificates WHERE name = ?`, name)
		if err != nil {
			return err
		}
		if !taken {
			break
		}
		name = fmt.Sprintf("%s-%d", validated.name, suffix)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO credential_certificates (id, name, kind, certificate_pem, private_key_pem, fingerprint, active, created_at, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		newID(), name, string(KindAPICertificate), validated.certificatePEM, validated.privateKeyPEM,
		certgen.Fingerprint(validated.chain[0]), !activeExists, store.now().UTC().Unix(), SourceMigrated)
	return err
}

func (store *Store) importRecorded(ctx context.Context, source string) (bool, error) {
	var found int
	err := store.handle.Reader().QueryRowContext(ctx, `SELECT 1 FROM credential_imports WHERE source = ?`, source).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (store *Store) ActiveAPICertificatePEM(ctx context.Context) ([]byte, bool, error) {
	var certificatePEM, privateKeyPEM string
	err := store.handle.Reader().QueryRowContext(ctx, `SELECT certificate_pem, private_key_pem FROM credential_certificates WHERE kind = ? AND active = 1`,
		string(KindAPICertificate)).Scan(&certificatePEM, &privateKeyPEM)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return []byte(certificatePEM + privateKeyPEM), true, nil
}
