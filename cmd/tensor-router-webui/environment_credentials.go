package main

import (
	"os"

	"tensors-router/internal/credential"
)

const (
	webUIAdminTokenEnvironment          = "TENSORS_ROUTER_WEBUI_ADMIN_TOKEN"
	webUIAdminTokenFileEnvironment      = "TENSORS_ROUTER_WEBUI_ADMIN_TOKEN_FILE"
	managedRouterTokenEnvironment       = "TENSORS_ROUTER_MANAGED_ROUTER_TOKEN"
	managedRouterTokenFileEnvironment   = "TENSORS_ROUTER_MANAGED_ROUTER_TOKEN_FILE"
	legacyWebUITokenEnvironment         = "TENSORS_ROUTER_WEBUI_TOKEN"
	legacySingularWebUITokenEnvironment = "TENSOR_ROUTER_WEBUI_TOKEN"
)

func webUICredentialOverrides(adminCLI string, routerCLI string) (string, string, error) {
	adminToken, err := webUICredentialOverride(adminCLI, credential.Source{
		Role:      "webui-admin",
		ValueName: webUIAdminTokenEnvironment,
		Value: firstNonEmpty(
			os.Getenv(webUIAdminTokenEnvironment),
			os.Getenv(legacyWebUITokenEnvironment),
			os.Getenv(legacySingularWebUITokenEnvironment),
		),
		FileName: webUIAdminTokenFileEnvironment,
		FilePath: os.Getenv(webUIAdminTokenFileEnvironment),
	})
	if err != nil {
		return "", "", err
	}
	routerToken, err := webUICredentialOverride(routerCLI, credential.Source{
		Role:      "managed-router",
		ValueName: managedRouterTokenEnvironment,
		Value:     os.Getenv(managedRouterTokenEnvironment),
		FileName:  managedRouterTokenFileEnvironment,
		FilePath:  os.Getenv(managedRouterTokenFileEnvironment),
	})
	if err != nil {
		return "", "", err
	}
	return adminToken, routerToken, nil
}

func webUICredentialOverride(commandLineValue string, environmentSource credential.Source) (string, error) {
	value, present, err := credential.Resolve(credential.Source{
		Role:      environmentSource.Role,
		ValueName: "command-line credential",
		Value:     commandLineValue,
	})
	if err != nil || present {
		return value, err
	}
	value, _, err = credential.Resolve(environmentSource)
	return value, err
}
