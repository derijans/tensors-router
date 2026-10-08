package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tensors-router/internal/flatyaml"
)

var routerConfigDialect = flatyaml.Dialect{
	StripComment:   flatyaml.StripTrailingComment,
	Scalar:         flatyaml.UnquoteScalar,
	SplitListItems: flatyaml.SplitOnUnquotedCommas,
}

func Load(path string) (Config, error) {
	return LoadWithOptions(path, LoadOptions{})
}

func LoadWithOptions(path string, options LoadOptions) (Config, error) {
	cfg := Defaults()
	if strings.TrimSpace(path) == "" {
		return cfg, fmt.Errorf("router configuration path is required")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := flatyaml.Decode(content, routerConfigDialect, cfg.schema()); err != nil {
		return cfg, err
	}
	if !filepath.IsAbs(cfg.MCP.Directory) {
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return cfg, err
		}
		cfg.MCP.Directory = filepath.Join(filepath.Dir(absolutePath), cfg.MCP.Directory)
	}
	if strings.TrimSpace(options.SecurityProfile) != "" {
		cfg.Security.Profile = strings.TrimSpace(options.SecurityProfile)
	}
	finalizeCompatibility(&cfg)
	if value := strings.TrimSpace(options.InferenceKey); value != "" {
		cfg.Auth.InferenceKeys = []string{value}
	}
	if value := strings.TrimSpace(options.AdminKey); value != "" {
		cfg.Auth.AdminKeys = []string{value}
	}
	if value := strings.TrimSpace(options.ClusterToken); value != "" {
		cfg.Cluster.Token = value
	}

	return cfg, validate(&cfg)
}
