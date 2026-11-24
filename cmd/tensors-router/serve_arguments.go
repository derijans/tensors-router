package main

import (
	"flag"
	"os"

	"tensors-router/internal/config"
	"tensors-router/internal/credentialstore"
)

type serveArguments struct {
	configPath           string
	trustBundlePath      string
	apiCertificateExport string
	webUICertificate     credentialstore.APICertificateFiles
	profileOverridden    bool
}

func loadServeConfig(args []string) (serveArguments, config.Config, error) {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := flags.String("config", "config.yaml", "config file")
	securityProfile := flags.String("security-profile", "", "security profile")
	trustBundlePath := flags.String("tls-trust-bundle", "", "write the certificates that verify this router's HTTPS listener to this file")
	apiCertificateExport := flags.String("api-certificate-export", "", "write the active API certificate chain and private key to this file (mode 0600) for the WebUI listener")
	webUICertFile := flags.String("import-api-cert", "", "certificate file the WebUI used so far; imported once as the API certificate")
	webUIKeyFile := flags.String("import-api-key", "", "private key file for --import-api-cert")
	if err := flags.Parse(args); err != nil {
		return serveArguments{}, config.Config{}, err
	}

	profileOverride := config.ResolveSecurityProfile(*securityProfile, os.Getenv("TENSORS_ROUTER_SECURITY_PROFILE"))
	loadOptions, err := environmentLoadOptions(profileOverride)
	if err != nil {
		return serveArguments{}, config.Config{}, err
	}
	cfg, err := config.LoadWithOptions(*configPath, loadOptions)
	arguments := serveArguments{
		configPath:           *configPath,
		trustBundlePath:      *trustBundlePath,
		apiCertificateExport: *apiCertificateExport,
		webUICertificate:     credentialstore.APICertificateFiles{Source: "webui", CertFile: *webUICertFile, KeyFile: *webUIKeyFile},
		profileOverridden:    profileOverride != "",
	}
	return arguments, cfg, err
}
