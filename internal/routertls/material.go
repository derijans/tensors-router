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
	built := &material{
		revision: snapshot.Revision,
		server: &tls.Config{
			MinVersion: tls.VersionTLS12,
			ClientAuth: tls.VerifyClientCertIfGiven,
			ClientCAs:  anchorPool(snapshot.TrustAnchors),
		},
		client: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    peerRoots(snapshot.TrustAnchors),
		},
	}
	if snapshot.Identity != nil {
		built.server.Certificates = []tls.Certificate{*snapshot.Identity}
		built.client.Certificates = []tls.Certificate{*snapshot.Identity}
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
