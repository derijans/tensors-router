package main

import (
	"errors"

	"tensors-router/internal/config"
	"tensors-router/internal/proxy"
)

var errProfileSetOutsideConfigFile = errors.New("the security profile is set by --security-profile or TENSORS_ROUTER_SECURITY_PROFILE; change it there")

func (runtime *serveRuntime) securityProfileControl() proxy.SecurityProfileControl {
	return proxy.SecurityProfileControl{
		Running:         runtime.cfg.Security.Profile,
		SecureBlocker:   runtime.secureProfileBlocker,
		WriteSecureFile: runtime.writeSecureProfile,
	}
}

func (runtime *serveRuntime) secureProfileBlocker() error {
	if runtime.profileOverridden {
		return errProfileSetOutsideConfigFile
	}
	return runtime.cfg.SecureProfileBlocker(storedCredentialCoverage(runtime.credentials.live.Current()))
}

func (runtime *serveRuntime) writeSecureProfile() error {
	return config.WriteSecurityProfile(runtime.configPath, config.SecurityProfileSecure)
}
