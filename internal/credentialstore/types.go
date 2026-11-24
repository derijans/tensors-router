package credentialstore

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Role string

const (
	RoleInference Role = "inference"
	RoleAdmin     Role = "admin"
	RoleMetrics   Role = "metrics"
	RoleCluster   Role = "cluster"
)

var Roles = []Role{RoleInference, RoleAdmin, RoleMetrics, RoleCluster}

type Kind string

const (
	KindIdentity         Kind = "identity"
	KindAuthority        Kind = "authority"
	KindTrustedAuthority Kind = "trusted_authority"
	KindPinnedPeer       Kind = "pinned_peer"
	KindAPICertificate   Kind = "api_certificate"
)

var Kinds = []Kind{KindIdentity, KindAuthority, KindTrustedAuthority, KindPinnedPeer, KindAPICertificate}

const (
	SourceCreated  = "created"
	SourceSupplied = "supplied"
	SourceMigrated = "migrated"
	SourceIssued   = "issued"
	SourceMaster   = "master"
)

type Token struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Role      Role      `json:"role"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

type Certificate struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Kind        Kind      `json:"kind"`
	Subject     string    `json:"subject"`
	Issuer      string    `json:"issuer"`
	DNSNames    []string  `json:"dns_names"`
	IPAddresses []string  `json:"ip_addresses"`
	Fingerprint string    `json:"fingerprint"`
	NotBefore   time.Time `json:"not_before"`
	NotAfter    time.Time `json:"not_after"`
	Active      bool      `json:"active"`
	Source      string    `json:"source"`
	CreatedAt   time.Time `json:"created_at"`
}

var (
	ErrNotFound = errors.New("credential not found")
	ErrConflict = errors.New("credential conflicts with an existing entry")
)

type InputError struct {
	message string
}

func (err InputError) Error() string { return err.message }

func inputError(format string, arguments ...any) error {
	return InputError{message: fmt.Sprintf(format, arguments...)}
}

func ParseRole(value string) (Role, error) {
	normalized := Role(strings.ToLower(strings.TrimSpace(value)))
	for _, role := range Roles {
		if role == normalized {
			return role, nil
		}
	}
	return "", inputError("role must be one of inference, admin, metrics, cluster")
}

func ParseKind(value string) (Kind, error) {
	normalized := Kind(strings.ToLower(strings.TrimSpace(value)))
	for _, kind := range Kinds {
		if kind == normalized {
			return kind, nil
		}
	}
	return "", inputError("kind must be one of identity, authority, trusted_authority, pinned_peer, api_certificate")
}

func (kind Kind) Activatable() bool {
	return kind == KindIdentity || kind == KindAPICertificate
}

func (kind Kind) holdsPrivateKey() bool {
	return kind == KindIdentity || kind == KindAuthority || kind == KindAPICertificate
}

func (kind Kind) isTrustAnchor() bool {
	return kind == KindAuthority || kind == KindTrustedAuthority || kind == KindPinnedPeer
}

func normalizedName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", inputError("name is required")
	}
	if len(name) > 128 {
		return "", inputError("name must be at most 128 characters")
	}
	return name, nil
}
