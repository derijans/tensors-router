package webui

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"os"
	"sync"

	"tensors-router/internal/certgen"
)

var systemTrustRoots *x509.CertPool

type routerTrust struct {
	path         string
	pinnedBySelf bool
	mu           sync.Mutex
	stamp        fileStamp
	transport    *http.Transport
}

func newRouterTrust(path string) *routerTrust {
	return &routerTrust{path: path}
}

func newManagedRouterTrust(path string) *routerTrust {
	return &routerTrust{path: path, pinnedBySelf: true}
}

func (trust *routerTrust) bundlePresent() bool {
	return trust.currentStamp().present
}

func (trust *routerTrust) currentStamp() fileStamp {
	return statFile(trust.path)
}

func (trust *routerTrust) RoundTrip(request *http.Request) (*http.Response, error) {
	transport, err := trust.transportForBundle()
	if err != nil {
		return nil, err
	}
	return transport.RoundTrip(request)
}

func (trust *routerTrust) transportForBundle() (*http.Transport, error) {
	stamp := trust.currentStamp()
	trust.mu.Lock()
	defer trust.mu.Unlock()
	if trust.transport != nil && trust.stamp == stamp {
		return trust.transport, nil
	}
	roots, err := trust.roots(stamp)
	if err != nil {
		return nil, err
	}
	next := http.DefaultTransport.(*http.Transport).Clone()
	next.TLSClientConfig = trust.clientConfig(roots, stamp.present)
	if trust.transport != nil {
		trust.transport.CloseIdleConnections()
	}
	trust.transport = next
	trust.stamp = stamp
	return next, nil
}

func (trust *routerTrust) clientConfig(roots *x509.CertPool, bundlePresent bool) *tls.Config {
	if !trust.pinnedBySelf || !bundlePresent {
		return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	}
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
		VerifyConnection:   verifyChainWithoutHostname(roots),
	}
}

func verifyChainWithoutHostname(roots *x509.CertPool) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) == 0 {
			return errors.New("router presented no certificate")
		}
		intermediates := x509.NewCertPool()
		for _, certificate := range state.PeerCertificates[1:] {
			intermediates.AddCert(certificate)
		}
		_, err := state.PeerCertificates[0].Verify(x509.VerifyOptions{
			Roots:         roots,
			Intermediates: intermediates,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		})
		return err
	}
}

func (trust *routerTrust) roots(stamp fileStamp) (*x509.CertPool, error) {
	if !stamp.present {
		return systemTrustRoots, nil
	}
	content, err := os.ReadFile(trust.path)
	if err != nil {
		return nil, err
	}
	certificates, err := certgen.ParseCertificates(content)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	for _, certificate := range certificates {
		pool.AddCert(certificate)
	}
	return pool, nil
}
