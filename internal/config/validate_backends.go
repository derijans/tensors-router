package config

import (
	"fmt"
	"net"
	"strings"

	"tensors-router/internal/backendendpoint"
)

// validateBackendEndpointUniqueness checks the six locally managed backend
// endpoints (kobold text/embeddings, llama text/embeddings, sdcpp,
// whispercpp) plus the router's own listener for two failure modes that
// otherwise surface only as a mysterious health-check timeout or, worse, one
// backend silently talking to another's process:
//
//   - a pinned port shared by two of these addresses
//   - a URL with no port at all, which earlier versions of this router
//     silently aliased onto a hardcoded default shared by every manager
//
// All six are checked unconditionally because they are all constructed at
// startup regardless of backend.mode. A port of 0 is exempt: it means "let
// the router allocate one at spawn time," which cannot collide by
// construction.
func validateBackendEndpointUniqueness(cfg *Config) error {
	candidates := []struct {
		key string
		raw string
	}{
		{"kobold.backend_url", cfg.Kobold.BackendURL},
		{"kobold.embeddings_backend_url", cfg.Kobold.EmbeddingsBackendURL},
		{"llama.backend_url", cfg.Llama.BackendURL},
		{"llama.embeddings_backend_url", cfg.Llama.EmbeddingsBackendURL},
		{"sdcpp.backend_url", cfg.SDCPP.BackendURL},
		{"whispercpp.backend_url", cfg.WhisperCPP.BackendURL},
	}

	seen := make(map[string]string, len(candidates)+1)
	if bindAddress, ok := normalizedLoopbackBindAddress(cfg.Server.Bind); ok {
		seen[bindAddress] = "server.bind"
	}

	for _, candidate := range candidates {
		address, pinned, err := pinnedBackendAddress(candidate.key, candidate.raw)
		if err != nil {
			return err
		}
		if !pinned {
			continue
		}
		if owner, exists := seen[address]; exists {
			return fmt.Errorf("%s and %s must not use the same address (%s)", owner, candidate.key, address)
		}
		seen[address] = candidate.key
	}
	return nil
}

const dynamicallyAllocatedPort = "0"

func pinnedBackendAddress(key string, raw string) (string, bool, error) {
	parsed, unparseable := backendendpoint.ParseLoopback(raw)
	if unparseable != nil {
		return "", false, nil
	}
	switch port := parsed.Port(); port {
	case "":
		return "", false, fmt.Errorf("%s must include an explicit port, or 0 to allocate one at startup", key)
	case dynamicallyAllocatedPort:
		return "", false, nil
	default:
		return normalizedBackendAddress(parsed.Hostname(), port), true, nil
	}
}

func normalizedLoopbackBindAddress(bind string) (string, bool) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(bind))
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(host, "localhost") {
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return "", false
		}
	}
	return normalizedBackendAddress(host, port), true
}

// normalizedBackendAddress folds only the "localhost" alias into
// 127.0.0.1; distinct loopback IPs (127.0.0.1 vs 127.0.0.2) are kept
// distinct since they genuinely do not collide.
func normalizedBackendAddress(host, port string) string {
	if strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
	}
	return strings.ToLower(host) + ":" + port
}

func validateNativeServerConfig(section string, server NativeServerConfig) error {
	if _, err := backendendpoint.ParseLoopback(server.BackendURL); err != nil {
		return fmt.Errorf("%s.backend_url is invalid: %w", section, err)
	}
	if err := backendendpoint.RejectConflictingArgs(server.ExtraArgs, "--host", "--port", "--listen-ip", "--listen-port"); err != nil {
		return fmt.Errorf("%s.%w", section, err)
	}
	if server.BinaryPath == "" {
		return fmt.Errorf("%s.binary_path is required", section)
	}
	if server.DataDir == "" {
		return fmt.Errorf("%s.data_dir is required", section)
	}
	return nil
}

func validateWhisperServerConfig(server NativeServerConfig) error {
	if err := validateNativeServerConfig("whispercpp", server); err != nil {
		return err
	}
	if err := backendendpoint.RejectConflictingArgs(server.ExtraArgs, "--public", "--public-path", "--request-path", "--inference-path", "--convert", "--no-convert", "--tmp-dir"); err != nil {
		return fmt.Errorf("whispercpp.%w", err)
	}
	return nil
}

func validateKoboldEndpoints(cfg *Config) error {
	if _, err := backendendpoint.ParseLoopback(cfg.Kobold.BackendURL); err != nil {
		return fmt.Errorf("kobold.backend_url is invalid: %w", err)
	}
	if _, err := backendendpoint.ParseLoopback(cfg.Kobold.EmbeddingsBackendURL); err != nil {
		return fmt.Errorf("kobold.embeddings_backend_url is invalid: %w", err)
	}
	if err := backendendpoint.RejectConflictingArgs(cfg.Kobold.ExtraArgs, "--host", "--port"); err != nil {
		return fmt.Errorf("kobold.%w", err)
	}
	return nil
}

func validateKoboldProcess(cfg *Config) error {
	if cfg.Kobold.BinaryPath == "" {
		return fmt.Errorf("kobold.binary_path is required")
	}
	if cfg.Kobold.DataDir == "" {
		return fmt.Errorf("kobold.data_dir is required")
	}
	if cfg.Kobold.Multiuser < 1 {
		return fmt.Errorf("kobold.multiuser must be at least 1")
	}
	return nil
}

func validateSplitBackendServers(cfg *Config) error {
	if cfg.Backend.Mode != "llama_sdcpp" {
		return nil
	}
	if err := validateNativeServerConfig("llama", cfg.Llama); err != nil {
		return err
	}
	if _, err := backendendpoint.ParseLoopback(cfg.Llama.EmbeddingsBackendURL); err != nil {
		return fmt.Errorf("llama.embeddings_backend_url is invalid: %w", err)
	}
	if err := validateNativeServerConfig("sdcpp", cfg.SDCPP); err != nil {
		return err
	}
	return validateWhisperServerConfig(cfg.WhisperCPP)
}
