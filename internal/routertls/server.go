package routertls

import "crypto/tls"

func (runtime *Runtime) ServerConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return runtime.current().server, nil
		},
	}
}
