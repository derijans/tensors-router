package routertls

import "crypto/tls"

type ListenerRole int

const (
	SharedListener ListenerRole = iota
	APIListener
	ClusterListener
)

func (runtime *Runtime) ServerConfig() *tls.Config {
	return runtime.ListenerConfig(SharedListener)
}

func (runtime *Runtime) ListenerConfig(role ListenerRole) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return runtime.current().serverFor(role), nil
		},
	}
}

func (runtime *Runtime) APIListenerServesTLS(identityFallback bool) bool {
	snapshot := runtime.source.Current()
	return snapshot.APICertificate != nil || identityFallback && snapshot.TLSEnabled()
}

func (built *material) serverFor(role ListenerRole) *tls.Config {
	switch role {
	case APIListener:
		return built.api
	case ClusterListener:
		return built.cluster
	default:
		return built.server
	}
}
