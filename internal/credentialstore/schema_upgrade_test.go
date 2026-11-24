package credentialstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"tensors-router/internal/certgen"
)

func TestVersionTwoCertificateTableGainsSourcesAndPendingKeys(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), Filename)
	writeVersionTwoStore(t, path)
	store := openTestStore(t, path)
	certificates, err := store.Certificates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, certificate := range certificates {
		sources[certificate.Name] = certificate.Source
	}
	if sources["migrated-router-config"] != SourceMigrated || sources["uploaded"] != SourceSupplied {
		t.Fatalf("sources after upgrade = %v; want the config import marked migrated and the rest supplied", sources)
	}
	if _, err := store.CreatePendingKey(ctx, PendingKeyRequest{Name: "next", Kind: KindAPICertificate, Hosts: []string{"api.lan"}}); err != nil {
		t.Fatalf("pending key after upgrade: %v", err)
	}
}

func writeVersionTwoStore(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE credential_certificates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL CHECK (kind IN ('identity', 'authority', 'trusted_authority', 'pinned_peer', 'api_certificate')),
			certificate_pem TEXT NOT NULL,
			private_key_pem TEXT,
			fingerprint TEXT NOT NULL UNIQUE,
			active INTEGER NOT NULL DEFAULT 0 CHECK (active IN (0, 1)),
			created_at INTEGER NOT NULL,
			CHECK (active = 0 OR kind IN ('identity', 'api_certificate')),
			CHECK ((kind IN ('identity', 'authority', 'api_certificate')) = (private_key_pem IS NOT NULL))
		)`); err != nil {
		t.Fatal(err)
	}
	for index, name := range []string{"migrated-router-config", "uploaded"} {
		keyPair, err := certgen.SelfSigned(certgen.SubjectFromHosts(name, []string{"api.lan"}), 24*time.Hour, certgen.ServerUsages)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO credential_certificates VALUES (?, ?, 'api_certificate', ?, ?, ?, ?, 0)`,
			name, name, string(keyPair.CertificatePEM), string(keyPair.PrivateKeyPEM), certgen.Fingerprint(keyPair.Certificate), index == 0); err != nil {
			t.Fatal(err)
		}
	}
}
