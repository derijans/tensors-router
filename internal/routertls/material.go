package routertls

import (
	"crypto/tls"
	"crypto/x509"
	"sync/atomic"

	"tensors-router/internal/credentialstore"
)

type SnapshotSource interface {
	Current() *credentialstore.Snapshot
}

type material struct {
	revision int64
	server   *tls.Config
	api      *tls.Config
	cluster  *tls.Config
	client   *tls.Config
}

type Runtime struct {
	source SnapshotSource
	cached atomic.Pointer[material]
}

func NewRuntime(source SnapshotSource) *Runtime {
	return &Runtime{source: source}
}

func (runtime *Runtime) Enabled() bool {
	return runtime.source.Current().TLSEnabled()
}

func (runtime *Runtime) current() *material {
	snapshot := runtime.source.Current()
	if cached := runtime.cached.Load(); cached != nil && cached.revision == snapshot.Revision {
		return cached
	}
	built := buildMaterial(snapshot)
	runtime.cached.Store(built)
	return built
}

func buildMaterial(snapshot *credentialstore.Snapshot) *material {
	anchors := anchorPool(snapshot.TrustAnchors)
	built := &material{
		revision: snapshot.Revision,
		server:   &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.VerifyClientCertIfGiven, ClientCAs: anchors},
		api:      &tls.Config{MinVersion: tls.VersionTLS12},
		cluster:  &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: anchors},
		client:   &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: peerRoots(snapshot.TrustAnchors)},
	}
	if snapshot.Identity != nil {
		identity := []tls.Certificate{*snapshot.Identity}
		built.server.Certificates = identity
		built.api.Certificates = identity
		built.cluster.Certificates = identity
		built.client.Certificates = identity
	}
	if snapshot.APICertificate != nil {
		built.api.Certificates = []tls.Certificate{*snapshot.APICertificate}
	}
	return built
}

func peerRoots(anchors []*x509.Certificate) *x509.CertPool {
	if len(anchors) == 0 {
		return systemRoots
	}
	return anchorPool(anchors)
}

var systemRoots *x509.CertPool

func anchorPool(anchors []*x509.Certificate) *x509.CertPool {
	pool := x509.NewCertPool()
	for _, anchor := range anchors {
		pool.AddCert(anchor)
	}
	return pool
}
