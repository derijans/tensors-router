package credentialstore

import (
	"context"
	"crypto/x509"
	"errors"
	"slices"
	"testing"

	"tensors-router/internal/certgen"
)

func signedByTestAuthority(t *testing.T, kind Kind, requestPEM string) (string, *x509.Certificate) {
	t.Helper()
	ctx := context.Background()
	master := newTestStore(t)
	authority, err := master.CreateAuthority(ctx, "cluster-ca")
	if err != nil {
		t.Fatal(err)
	}
	chainPEM, err := master.SignCertificateRequest(ctx, authority.ID, kind, requestPEM)
	if err != nil {
		t.Fatal(err)
	}
	authorityPEM, err := master.PublicCertificatePEM(ctx, authority.ID)
	if err != nil {
		t.Fatal(err)
	}
	authorityCertificates, err := certgen.ParseCertificates([]byte(authorityPEM))
	if err != nil {
		t.Fatal(err)
	}
	return chainPEM, authorityCertificates[0]
}

func TestSignedRequestInstallsAgainstThePendingKey(t *testing.T) {
	cases := []struct {
		kind       Kind
		wantClient bool
	}{
		{KindAPICertificate, false},
		{KindIdentity, true},
	}
	for _, testCase := range cases {
		ctx := context.Background()
		slave := newTestStore(t)
		pending, err := slave.CreatePendingKey(ctx, PendingKeyRequest{Name: "slave-a", Kind: testCase.kind, Hosts: []string{"slave-a.lan", "10.0.0.5"}})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(pending.DNSNames, []string{"slave-a.lan"}) || !slices.Equal(pending.IPAddresses, []string{"10.0.0.5"}) {
			t.Fatalf("pending hosts = %v %v", pending.DNSNames, pending.IPAddresses)
		}
		chainPEM, authority := signedByTestAuthority(t, testCase.kind, pending.RequestPEM)
		installed, err := slave.InstallSignedCertificate(ctx, SignedCertificateInstall{CertificatePEM: chainPEM, Activate: true, Source: SourceMaster})
		if err != nil {
			t.Fatal(err)
		}
		if installed.Kind != testCase.kind || !installed.Active || installed.Source != SourceMaster || installed.Name != "slave-a" {
			t.Fatalf("installed %#v", installed)
		}
		remaining, err := slave.PendingKeys(ctx)
		if err != nil || len(remaining) != 0 {
			t.Fatalf("pending keys after install = %v, %v; want none", remaining, err)
		}
		leaf := activeLeaf(t, slave, testCase.kind)
		roots := x509.NewCertPool()
		roots.AddCert(authority)
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			t.Fatalf("%s does not verify against the signing authority: %v", testCase.kind, err)
		}
		if allowsClient := slices.Contains(leaf.ExtKeyUsage, x509.ExtKeyUsageClientAuth); allowsClient != testCase.wantClient {
			t.Fatalf("%s client authentication = %t, want %t", testCase.kind, allowsClient, testCase.wantClient)
		}
	}
}

func activeLeaf(t *testing.T, store *Store, kind Kind) *x509.Certificate {
	t.Helper()
	snapshot, err := store.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	active := snapshot.Identity
	if kind == KindAPICertificate {
		active = snapshot.APICertificate
	}
	if active == nil {
		t.Fatalf("no active %s after install", kind)
	}
	leaf, err := x509.ParseCertificate(active.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return leaf
}

func TestInstallRejectsCertificateForAnotherKey(t *testing.T) {
	ctx := context.Background()
	slave := newTestStore(t)
	if _, err := slave.CreatePendingKey(ctx, PendingKeyRequest{Name: "slave-a", Kind: KindAPICertificate, Hosts: []string{"slave-a.lan"}}); err != nil {
		t.Fatal(err)
	}
	foreign, err := certgen.NewCertificateRequest(certgen.SubjectFromHosts("slave-a", []string{"slave-a.lan"}))
	if err != nil {
		t.Fatal(err)
	}
	chainPEM, _ := signedByTestAuthority(t, KindAPICertificate, string(foreign.RequestPEM))
	_, err = slave.InstallSignedCertificate(ctx, SignedCertificateInstall{CertificatePEM: chainPEM, Activate: true})
	var input InputError
	if !errors.As(err, &input) {
		t.Fatalf("install of a certificate for a foreign key = %v; want an input error", err)
	}
	certificates, err := slave.Certificates(ctx)
	if err != nil || len(certificates) != 0 {
		t.Fatalf("certificates after rejected install = %v, %v; want none", certificates, err)
	}
	pending, err := slave.PendingKeys(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending keys after rejected install = %v, %v; want the original one", pending, err)
	}
}

func TestPendingKeyNameMustNotClashWithACertificate(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	if _, err := store.GenerateIdentity(ctx, "node", []string{"node.lan"}); err != nil {
		t.Fatal(err)
	}
	_, err := store.CreatePendingKey(ctx, PendingKeyRequest{Name: "node", Kind: KindIdentity, Hosts: []string{"node.lan"}})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("pending key named like a certificate = %v; want ErrConflict", err)
	}
}

func TestCertificateRequestsOnlyForActivatableKinds(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	_, err := store.CreatePendingKey(ctx, PendingKeyRequest{Name: "ca", Kind: KindAuthority, Hosts: []string{"ca.lan"}})
	var input InputError
	if !errors.As(err, &input) {
		t.Fatalf("authority certificate request = %v; want an input error", err)
	}
}
