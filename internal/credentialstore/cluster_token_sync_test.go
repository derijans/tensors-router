package credentialstore

import (
	"context"
	"testing"
)

func TestEnsureClusterTokenStoresEachSecretOnce(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	first, created, err := store.EnsureClusterToken(ctx, "cluster-from-master", "master-cluster-token-0123456789")
	if err != nil || !created {
		t.Fatalf("first sync = %t, %v; want created", created, err)
	}
	again, created, err := store.EnsureClusterToken(ctx, "cluster-from-master", "master-cluster-token-0123456789")
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("repeated sync = %#v, %t, %v; want the existing token", again, created, err)
	}
	rotated, created, err := store.EnsureClusterToken(ctx, "cluster-from-master", "master-cluster-token-rotated-0123")
	if err != nil || !created || rotated.Name != "cluster-from-master-2" {
		t.Fatalf("rotated sync = %#v, %t, %v; want a second token with a free name", rotated, created, err)
	}
	snapshot, err := store.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.OutboundClusterToken != "master-cluster-token-rotated-0123" {
		t.Fatalf("outbound cluster token = %q; want the newest synced secret", snapshot.OutboundClusterToken)
	}
}

func TestEnsureClusterTokenRefusesASecretHeldByAnotherRole(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	if _, _, err := store.CreateToken(ctx, NewToken{Name: "admin", Role: RoleAdmin, Supplied: "shared-secret-value-0123456789"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.EnsureClusterToken(ctx, "cluster-from-master", "shared-secret-value-0123456789"); err == nil {
		t.Fatal("cluster token sync accepted a secret already stored as an admin token")
	}
}
