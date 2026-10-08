package webui

import "tensors-router/internal/flatyaml"

var webUIConfigDialect = flatyaml.Dialect{
	Scalar:         flatyaml.UnquoteScalar,
	SplitListItems: flatyaml.SplitOnEveryComma,
}

func (cfg *Config) schema() flatyaml.Schema {
	return flatyaml.Schema{
		"security": {Fields: flatyaml.Fields{
			"profile": flatyaml.String(&cfg.Security.Profile),
		}},
		"server": {Fields: flatyaml.Fields{
			"bind":                  flatyaml.String(&cfg.Server.Bind),
			"backend_ui_bind":       flatyaml.String(&cfg.Server.BackendUIBind),
			"backend_ui_public_url": flatyaml.String(&cfg.Server.BackendUIPublicURL),
			"state_dir":             flatyaml.String(&cfg.Server.StateDir),
			"cert_file":             flatyaml.String(&cfg.Server.CertFile),
			"key_file":              flatyaml.String(&cfg.Server.KeyFile),
			"admin_token":           flatyaml.String(&cfg.Server.AdminToken),
			"cert_hosts":            flatyaml.StringList(&cfg.Server.CertHosts),
		}},
		"router": {Fields: flatyaml.Fields{
			"url":                 flatyaml.String(&cfg.Router.URL),
			"token":               flatyaml.String(&cfg.Router.Token),
			"binary_path":         flatyaml.String(&cfg.Router.BinaryPath),
			"config_path":         flatyaml.String(&cfg.Router.ConfigPath),
			"start_when_missing":  flatyaml.Bool(&cfg.Router.StartWhenMissing),
			"shutdown_with_webui": flatyaml.Bool(&cfg.Router.ShutdownWithWebUI),
			"args":                flatyaml.StringList(&cfg.Router.Args),
		}},
		"logging": {Fields: flatyaml.Fields{
			"mode":    flatyaml.String(&cfg.Logging.Mode).Marking(&cfg.Logging.modeSet),
			"enabled": flatyaml.Bool(&cfg.Logging.Enabled).Marking(&cfg.Logging.legacyEnabledSet),
		}},
	}
}
