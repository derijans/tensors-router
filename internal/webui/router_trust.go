package webui

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"sync"
	"time"

	"tensors-router/internal/certgen"
)

type trustFileStamp struct {
	present bool
	size    int64
	modTime time.Time
}

var systemTrustRoots *x509.CertPool

type routerTrust struct {
	path      string
	mu        sync.Mutex
	stamp     trustFileStamp
	transport *http.Transport
}

func newRouterTrust(path string) *routerTrust {
	return &routerTrust{path: path}
}

func (trust *routerTrust) bundlePresent() bool {
	return trust.currentStamp().present
}

func (trust *routerTrust) currentStamp() trustFileStamp {
	if trust.path == "" {
		return trustFileStamp{}
	}
	info, err := os.Stat(trust.path)
	if err != nil {
		return trustFileStamp{}
	}
	return trustFileStamp{present: true, size: info.Size(), modTime: info.ModTime()}
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
	next.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	if trust.transport != nil {
		trust.transport.CloseIdleConnections()
	}
	trust.transport = next
	trust.stamp = stamp
	return next, nil
}

func (trust *routerTrust) roots(stamp trustFileStamp) (*x509.CertPool, error) {
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
