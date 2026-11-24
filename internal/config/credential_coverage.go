package config

import "fmt"

type StoredCredentials struct {
	Inference bool
	Admin     bool
	Cluster   bool
}

const tokenCommandHint = "create one with `tensors-router security token create`"

func (cfg Config) RequiresProtectedTokens() (inference bool, admin bool, cluster bool) {
	nonLoopbackSecure := cfg.Security.Profile == SecurityProfileSecure && !loopbackBind(cfg.Server.Bind)
	return nonLoopbackSecure, nonLoopbackSecure || cfg.MCP.Enabled, cfg.Cluster.Role != "standalone"
}

func (cfg Config) CheckStoredCredentials(stored StoredCredentials) ([]string, error) {
	inference, admin, cluster := cfg.RequiresProtectedTokens()
	if inference && !stored.Inference {
		return nil, fmt.Errorf("an inference token is required for non-loopback secure bind; %s", tokenCommandHint)
	}
	if admin && !stored.Admin {
		return nil, fmt.Errorf("an admin token is required for non-loopback secure bind and for MCP; %s", tokenCommandHint)
	}
	if cluster && !stored.Cluster {
		return nil, fmt.Errorf("a cluster token is required when cluster.role is not standalone; %s", tokenCommandHint)
	}
	if cfg.Security.Profile == SecurityProfileSecure && !stored.Inference && !stored.Admin {
		return []string{"security.profile is secure but no inference or admin tokens exist; the admin API is open to any local process"}, nil
	}
	return nil, nil
}

func (cfg Config) ConfiguredTokenSettings() []string {
	var present []string
	for _, role := range credentialRoles(&cfg) {
		if hasCredential(role.values) {
			present = append(present, role.name)
		}
	}
	return present
}
