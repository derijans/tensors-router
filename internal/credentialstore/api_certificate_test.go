package credentialstore

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tensors-router/internal/certgen"
)

func apiCertificateFiles(t *testing.T, hosts ...string) APICertificateFiles {
	t.Helper()
	authority, err := certgen.Authority("public-ca", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := certgen.Issue(authority, certgen.SubjectFromHosts("api", hosts), 24*time.Hour, certgen.ServerUsages)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	files := APICertificateFiles{Source: "webui", CertFile: filepath.Join(directory, "api.crt"), KeyFile: filepath.Join(directory, "api.key")}
	if err := os.WriteFile(files.CertFile, append(issued.CertificatePEM, authority.CertificatePEM...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files.KeyFile, issued.PrivateKeyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestAPICertificateAndIdentityStayActiveSideBySide(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	identity, err := store.GenerateIdentity(ctx, "node1", []string{"node1.lan"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Activate(ctx, identity.ID); err != nil {
		t.Fatal(err)
	}
	imported, err := store.ImportAPICertificateFilesOnce(ctx, apiCertificateFiles(t, "api.example.com"))
	if err != nil || !imported {
		t.Fatalf("import = %t, %v", imported, err)
	}
	snapshot, err := store.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Identity == nil || snapshot.APICertificate == nil {
		t.Fatalf("identity %v, API certificate %v; want both active", snapshot.Identity != nil, snapshot.APICertificate != nil)
	}
	for _, anchor := range snapshot.TrustAnchors {
		if anchor.Equal(snapshot.APICertificateLeaf) {
			t.Fatal("the API certificate must not authenticate cluster peers")
		}
	}
}

func TestAPICertificateFilesImportOnceAndDeletionSticks(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	files := apiCertificateFiles(t, "api.example.com")
	if imported, err := store.ImportAPICertificateFilesOnce(ctx, files); err != nil || !imported {
		t.Fatalf("first import = %t, %v", imported, err)
	}
	if err := store.DeleteCertificate(ctx, "migrated-webui", true); err != nil {
		t.Fatal(err)
	}
	if imported, err := store.ImportAPICertificateFilesOnce(ctx, files); err != nil || imported {
		t.Fatalf("second import = %t, %v; want no import after deletion", imported, err)
	}
}

func TestAPICertificateRequiresServerAuthentication(t *testing.T) {
	authority, err := certgen.Authority("public-ca", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validateImport(CertificateImport{Name: "ca", Kind: KindAPICertificate, CertificatePEM: string(authority.CertificatePEM), PrivateKeyPEM: string(authority.PrivateKeyPEM)}, time.Now()); err == nil {
		t.Fatal("a certificate authority was accepted as an API certificate")
	}
}

func TestTrustBundleLetsTheWebUIVerifyTheAPICertificate(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	if _, err := store.ImportAPICertificateFilesOnce(ctx, apiCertificateFiles(t, "api.example.com")); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := store.TrustBundle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	anchors, err := certgen.ParseCertificates(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(anchors) != 1 || !anchors[0].Equal(snapshot.APICertificateLeaf) {
		t.Fatalf("trust bundle holds %d certificates; want the API certificate", len(anchors))
	}
}

func TestVersionOneCertificateTableUpgradesWithoutLosingTheActiveIdentity(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), Filename)
	writeVersionOneStore(t, path)
	store := openTestStore(t, path)
	snapshot, err := store.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Identity == nil {
		t.Fatal("active identity lost in the schema upgrade")
	}
	if imported, err := store.ImportAPICertificateFilesOnce(ctx, apiCertificateFiles(t, "api.example.com")); err != nil || !imported {
		t.Fatalf("API certificate after upgrade = %t, %v", imported, err)
	}
}

func writeVersionOneStore(t *testing.T, path string) {
	t.Helper()
	identity, err := certgen.SelfSigned(certgen.SubjectFromHosts("node1", []string{"node1.lan"}), 24*time.Hour, certgen.MutualTLSUsages)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	statements := []string{
		`CREATE TABLE credential_certificates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL CHECK (kind IN ('identity', 'authority', 'trusted_authority', 'pinned_peer')),
			certificate_pem TEXT NOT NULL,
			private_key_pem TEXT,
			fingerprint TEXT NOT NULL UNIQUE,
			active INTEGER NOT NULL DEFAULT 0 CHECK (active IN (0, 1)),
			created_at INTEGER NOT NULL,
			CHECK (active = 0 OR kind = 'identity'),
			CHECK ((kind IN ('identity', 'authority')) = (private_key_pem IS NOT NULL))
		)`,
		`CREATE UNIQUE INDEX credential_certificates_one_active_identity ON credential_certificates (active) WHERE active = 1`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO credential_certificates VALUES ('id-1', 'node1', 'identity', ?, ?, ?, 1, 0)`,
		string(identity.CertificatePEM), string(identity.PrivateKeyPEM), certgen.Fingerprint(identity.Certificate)); err != nil {
		t.Fatal(err)
	}
}
