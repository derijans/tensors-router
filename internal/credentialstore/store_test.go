package credentialstore

import (
	"context"
	"crypto/x509"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/certgen"
	"tensors-router/internal/credential"
)

func openTestStore(t *testing.T, path string, protected ...Role) *Store {
	t.Helper()
	store, err := Open(context.Background(), Config{Path: path, ProtectedRoles: protected})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newTestStore(t *testing.T, protected ...Role) *Store {
	t.Helper()
	return openTestStore(t, filepath.Join(t.TempDir(), Filename), protected...)
}

func TestConfiguredTokensImportOnceAndDeletionsStick(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), Filename)
	configured := ConfiguredCredentials{
		InferenceKeys: []string{"inference-key-0123456789"},
		AdminKeys:     []string{"admin-key-0123456789abcd"},
		ClusterToken:  "cluster-token-0123456789",
	}
	first, err := Open(ctx, Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	imported, err := first.ImportConfiguredOnce(ctx, configured)
	if err != nil || imported != 3 {
		t.Fatalf("first import = %d, %v; want 3", imported, err)
	}
	if err := first.DeleteToken(ctx, "migrated-inference-1"); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second := openTestStore(t, path)
	imported, err = second.ImportConfiguredOnce(ctx, configured)
	if err != nil || imported != 0 {
		t.Fatalf("second import = %d, %v; want 0", imported, err)
	}
	tokens, err := second.Tokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 2 {
		t.Fatalf("deleted migrated token came back: %#v", tokens)
	}
}

func TestTokenSecretsAreStoredAsHashesExceptClusterTokens(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, adminSecret, err := store.CreateToken(ctx, NewToken{Name: "ops", Role: RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(adminSecret, generatedTokenPrefix) {
		t.Fatalf("generated secret %q lacks prefix", adminSecret)
	}
	var stored int
	if err := store.handle.Reader().QueryRowContext(ctx, `SELECT COUNT(*) FROM credential_tokens WHERE cluster_secret = ? OR CAST(secret_hash AS TEXT) = ?`, adminSecret, adminSecret).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Fatal("admin token secret is stored in plaintext")
	}
	cluster, _, err := store.CreateToken(ctx, NewToken{Name: "mesh", Role: RoleCluster, Supplied: "shared-cluster-value-0123"})
	if err != nil {
		t.Fatal(err)
	}
	if secret, err := store.ClusterSecret(ctx, cluster.ID); err != nil || secret != "shared-cluster-value-0123" {
		t.Fatalf("cluster secret = %q, %v", secret, err)
	}
	if _, err := store.ClusterSecret(ctx, "ops"); !errors.As(err, &InputError{}) {
		t.Fatalf("admin secret was retrievable: %v", err)
	}
	snapshot, err := store.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !credential.MatchesAnyHash(adminSecret, snapshot.Keys.Admin) || snapshot.OutboundClusterToken != "shared-cluster-value-0123" {
		t.Fatalf("snapshot does not authorize stored tokens: %#v", snapshot)
	}
}

func TestTokenValueCannotBeReusedAcrossRoles(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	if _, _, err := store.CreateToken(ctx, NewToken{Name: "a", Role: RoleInference, Supplied: "same-value-0123456789"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateToken(ctx, NewToken{Name: "b", Role: RoleAdmin, Supplied: "same-value-0123456789"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("same secret accepted for a second role: %v", err)
	}
}

func TestLastProtectedTokenCannotBeDeleted(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t, RoleAdmin)
	if _, _, err := store.CreateToken(ctx, NewToken{Name: "only", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteToken(ctx, "only"); !errors.As(err, &InputError{}) {
		t.Fatalf("last admin token was deleted: %v", err)
	}
	if _, _, err := store.CreateToken(ctx, NewToken{Name: "replacement", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteToken(ctx, "only"); err != nil {
		t.Fatalf("delete with replacement present: %v", err)
	}
}

func TestOnlyOneIdentityIsActive(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	first, err := store.GenerateIdentity(ctx, "first", []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.GenerateIdentity(ctx, "second", []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first.ID, second.ID} {
		if err := store.Activate(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	certificates, err := store.Certificates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, certificate := range certificates {
		if certificate.Active {
			active++
			if certificate.ID != second.ID {
				t.Fatalf("wrong identity active: %s", certificate.Name)
			}
		}
	}
	if active != 1 {
		t.Fatalf("active identities = %d", active)
	}
	if err := store.DeleteCertificate(ctx, second.ID, false); !errors.As(err, &InputError{}) {
		t.Fatalf("active identity deleted without force: %v", err)
	}
}

func TestImportRejectsMismatchedKeyAndKeysOnTrustAnchors(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	subject := certgen.SubjectFromHosts("peer", []string{"127.0.0.1"})
	one, err := certgen.SelfSigned(subject, time.Hour, certgen.MutualTLSUsages)
	if err != nil {
		t.Fatal(err)
	}
	other, err := certgen.SelfSigned(subject, time.Hour, certgen.MutualTLSUsages)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ImportCertificate(ctx, CertificateImport{Name: "x", Kind: KindIdentity, CertificatePEM: string(one.CertificatePEM), PrivateKeyPEM: string(other.PrivateKeyPEM)})
	if !errors.As(err, &InputError{}) || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched key accepted: %v", err)
	}
	_, err = store.ImportCertificate(ctx, CertificateImport{Name: "y", Kind: KindPinnedPeer, CertificatePEM: string(one.CertificatePEM), PrivateKeyPEM: string(one.PrivateKeyPEM)})
	if !errors.As(err, &InputError{}) {
		t.Fatalf("private key stored for a pinned peer: %v", err)
	}
}

func TestIssuedIdentityVerifiesAgainstItsAuthority(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	authority, err := store.CreateAuthority(ctx, "cluster-ca")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := store.IssueIdentity(ctx, authority.ID, "node2", []string{"node2.lan"})
	if err != nil {
		t.Fatal(err)
	}
	chain, err := certgen.ParseCertificates([]byte(issued.CertificatePEM))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	authorityPEM, err := store.PublicCertificatePEM(ctx, authority.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !roots.AppendCertsFromPEM([]byte(authorityPEM)) {
		t.Fatal("authority export is not a certificate")
	}
	for _, usage := range certgen.MutualTLSUsages {
		if _, err := chain[0].Verify(x509.VerifyOptions{Roots: roots, DNSName: "node2.lan", KeyUsages: []x509.ExtKeyUsage{usage}}); err != nil {
			t.Fatalf("issued identity does not verify for %v: %v", usage, err)
		}
	}
	if strings.Contains(authorityPEM, "PRIVATE KEY") {
		t.Fatal("authority export leaked the private key")
	}
}

func TestEveryWriteAdvancesTheRevision(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	before, err := store.Revision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateToken(ctx, NewToken{Name: "t", Role: RoleMetrics}); err != nil {
		t.Fatal(err)
	}
	after, err := store.Revision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after <= before {
		t.Fatalf("revision did not advance: %d -> %d", before, after)
	}
}
