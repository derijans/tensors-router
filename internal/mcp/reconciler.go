package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"tensors-router/internal/atomicfile"
)

const (
	BackendKobold = "kobold"
	BackendLlama  = "llama_sdcpp"
)

type Config struct {
	Enabled   bool
	Directory string
	ConfigDir string
}

type Reconciler struct {
	enabled   bool
	directory string
	configDir string
	mu        sync.Mutex
}

type Result struct {
	Enabled     bool
	ServersPath string
	OverlayPath string
}

func (reconciler *Reconciler) Enabled() bool {
	return reconciler != nil && reconciler.enabled
}

type server struct {
	Name       string          `json:"name"`
	Definition json.RawMessage `json:"definition"`
}

func NewReconciler(config Config) (*Reconciler, error) {
	configDir, err := absoluteDirectory(config.ConfigDir)
	if err != nil {
		return nil, err
	}
	directory := strings.TrimSpace(config.Directory)
	if directory == "" {
		directory = "./mcp"
	}
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(configDir, directory)
	}
	directory, err = absoluteDirectory(directory)
	if err != nil {
		return nil, err
	}
	return &Reconciler{enabled: config.Enabled, directory: directory, configDir: configDir}, nil
}

func (reconciler *Reconciler) Reconcile(filename string, backend string) (Result, error) {
	reconciler.mu.Lock()
	defer reconciler.mu.Unlock()

	target, stem, err := reconciler.configTarget(filename)
	if err != nil {
		return Result{}, err
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return Result{}, err
	}
	servers, enabled, err := decodeServers(content, backend)
	if err != nil {
		return Result{}, err
	}
	if !reconciler.enabled || !enabled {
		return Result{}, reconciler.removeArtifacts(stem)
	}
	serversPath := filepath.Join(reconciler.directory, stem, "servers.json")
	generated, err := generatedServers(servers)
	if err != nil {
		return Result{}, err
	}
	if err := ensurePrivateDirectory(filepath.Dir(serversPath)); err != nil {
		return Result{}, err
	}
	if err := atomicfile.Write(serversPath, generated, 0o600); err != nil {
		return Result{}, err
	}
	result := Result{Enabled: true, ServersPath: serversPath}
	if backend == BackendKobold {
		overlayPath := filepath.Join(reconciler.configDir, ".router-mcp", filename)
		overlay, err := json.Marshal(map[string]string{"mcpfile": serversPath})
		if err != nil {
			return Result{}, err
		}
		if err := ensurePrivateDirectory(filepath.Dir(overlayPath)); err != nil {
			return Result{}, err
		}
		if err := atomicfile.Write(overlayPath, overlay, 0o600); err != nil {
			return Result{}, err
		}
		result.OverlayPath = overlayPath
	}
	return result, nil
}

func (reconciler *Reconciler) ReconcileAll(backend string) error {
	reconciler.mu.Lock()
	defer reconciler.mu.Unlock()
	entries, err := os.ReadDir(reconciler.configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".kcpps") || strings.HasPrefix(entry.Name(), ".router-mcp") {
			continue
		}
		if _, err := reconciler.reconcileLocked(entry.Name(), backend); err != nil {
			return err
		}
	}
	return nil
}

func (reconciler *Reconciler) Remove(filename string) error {
	reconciler.mu.Lock()
	defer reconciler.mu.Unlock()
	_, stem, err := reconciler.configTarget(filename)
	if err != nil {
		return err
	}
	return reconciler.removeArtifacts(stem)
}

func Validate(content []byte, fallbackBackend string) error {
	_, _, err := decodeServers(content, fallbackBackend)
	return err
}

func (reconciler *Reconciler) reconcileLocked(filename string, backend string) (Result, error) {
	target, stem, err := reconciler.configTarget(filename)
	if err != nil {
		return Result{}, err
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return Result{}, err
	}
	servers, enabled, err := decodeServers(content, backend)
	if err != nil {
		return Result{}, err
	}
	if !reconciler.enabled || !enabled {
		return Result{}, reconciler.removeArtifacts(stem)
	}
	serversPath := filepath.Join(reconciler.directory, stem, "servers.json")
	generated, err := generatedServers(servers)
	if err != nil {
		return Result{}, err
	}
	if err := ensurePrivateDirectory(filepath.Dir(serversPath)); err != nil {
		return Result{}, err
	}
	if err := atomicfile.Write(serversPath, generated, 0o600); err != nil {
		return Result{}, err
	}
	result := Result{Enabled: true, ServersPath: serversPath}
	if backend == BackendKobold {
		overlayPath := filepath.Join(reconciler.configDir, ".router-mcp", filename)
		overlay, err := json.Marshal(map[string]string{"mcpfile": serversPath})
		if err != nil {
			return Result{}, err
		}
		if err := ensurePrivateDirectory(filepath.Dir(overlayPath)); err != nil {
			return Result{}, err
		}
		if err := atomicfile.Write(overlayPath, overlay, 0o600); err != nil {
			return Result{}, err
		}
		result.OverlayPath = overlayPath
	}
	return result, nil
}

func (reconciler *Reconciler) removeArtifacts(stem string) error {
	if err := removeValidatedDirectory(reconciler.directory, stem); err != nil {
		return err
	}
	return removeValidatedFile(filepath.Join(reconciler.configDir, ".router-mcp"), stem+".kcpps")
}

func (reconciler *Reconciler) configTarget(filename string) (string, string, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" || filename != filepath.Base(filename) || !filepath.IsLocal(filename) || !strings.EqualFold(filepath.Ext(filename), ".kcpps") {
		return "", "", fmt.Errorf("config filename is invalid")
	}
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	if stem == "" {
		return "", "", fmt.Errorf("config filename is invalid")
	}
	return filepath.Join(reconciler.configDir, filename), stem, nil
}

func absoluteDirectory(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("directory is required")
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func removeValidatedDirectory(root string, name string) error {
	if name == "" || name != filepath.Base(name) || !filepath.IsLocal(name) {
		return fmt.Errorf("artifact directory is invalid")
	}
	target := filepath.Join(root, name)
	if filepath.Dir(target) != filepath.Clean(root) {
		return fmt.Errorf("artifact directory is invalid")
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("artifact directory is unsafe")
	}
	return os.RemoveAll(target)
}

func removeValidatedFile(root string, name string) error {
	if name == "" || name != filepath.Base(name) || !filepath.IsLocal(name) {
		return fmt.Errorf("artifact filename is invalid")
	}
	target := filepath.Join(root, name)
	if filepath.Dir(target) != filepath.Clean(root) {
		return fmt.Errorf("artifact filename is invalid")
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("artifact file is unsafe")
	}
	return os.Remove(target)
}

func decodeServers(content []byte, backend string) ([]server, bool, error) {
	value, err := decodeUniqueJSON(content)
	if err != nil {
		return nil, false, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("configuration must be a JSON object")
	}
	if _, legacy := root["mcpfile"]; legacy {
		return nil, false, fmt.Errorf("mcpfile is not supported; use mcp_servers")
	}
	if configuredBackend, exists := root["backend_mode"].(string); exists && configuredBackend != "" {
		backend = configuredBackend
	}
	if backend != BackendKobold && backend != BackendLlama {
		return nil, false, fmt.Errorf("backend mode is invalid")
	}
	rawEnabled, hasEnabled := root["mcp_enabled"]
	enabled := false
	if hasEnabled {
		var ok bool
		enabled, ok = rawEnabled.(bool)
		if !ok {
			return nil, false, fmt.Errorf("mcp_enabled must be a boolean")
		}
	}
	rawServers, exists := root["mcp_servers"]
	if !exists {
		return nil, enabled, nil
	}
	array, ok := rawServers.([]any)
	if !ok {
		return nil, false, fmt.Errorf("mcp_servers must be an array")
	}
	servers := make([]server, 0, len(array))
	seen := make(map[string]struct{}, len(array))
	for _, item := range array {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("mcp_servers entries must be objects")
		}
		name, ok := entry["name"].(string)
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, false, fmt.Errorf("mcp server name is required")
		}
		if _, exists := seen[name]; exists {
			return nil, false, fmt.Errorf("duplicate mcp server name %q", name)
		}
		definition, ok := entry["definition"].(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("mcp server %q definition must be an object", name)
		}
		if err := validateDefinition(definition, backend); err != nil {
			return nil, false, fmt.Errorf("mcp server %q: %w", name, err)
		}
		encoded, err := json.Marshal(definition)
		if err != nil {
			return nil, false, err
		}
		seen[name] = struct{}{}
		servers = append(servers, server{Name: name, Definition: encoded})
	}
	return servers, enabled, nil
}

func validateDefinition(definition map[string]any, backend string) error {
	command, hasCommand := definition["command"]
	url, hasURL := definition["url"]
	if hasCommand == hasURL {
		return fmt.Errorf("definition requires exactly one of command or url")
	}
	if hasCommand {
		if _, ok := command.(string); !ok || strings.TrimSpace(command.(string)) == "" {
			return fmt.Errorf("command must be a non-empty string")
		}
		if args, exists := definition["args"]; exists && !stringArray(args) {
			return fmt.Errorf("args must be an array of strings")
		}
		if env, exists := definition["env"]; exists && !stringMap(env) {
			return fmt.Errorf("env must be an object containing string values")
		}
		return nil
	}
	if backend != BackendKobold {
		return fmt.Errorf("HTTP(S) transport is not supported by this backend")
	}
	address, ok := url.(string)
	if !ok || !(strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://")) {
		return fmt.Errorf("url must be an HTTP(S) URL")
	}
	if headers, exists := definition["headers"]; exists && !stringMap(headers) {
		return fmt.Errorf("headers must be an object containing string values")
	}
	return nil
}

func stringArray(value any) bool {
	values, ok := value.([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

func stringMap(value any) bool {
	values, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if _, ok := value.(string); !ok {
			return false
		}
	}
	return true
}

func generatedServers(servers []server) ([]byte, error) {
	sort.Slice(servers, func(left, right int) bool { return servers[left].Name < servers[right].Name })
	definitions := make(map[string]json.RawMessage, len(servers))
	for _, server := range servers {
		definitions[server.Name] = server.Definition
	}
	return json.Marshal(map[string]any{"mcpServers": definitions})
}

func decodeUniqueJSON(content []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	value, err := decodeUniqueValue(decoder)
	if err != nil {
		return nil, err
	}
	if token, err := decoder.Token(); err != io.EOF || token != nil {
		return nil, fmt.Errorf("configuration contains trailing JSON")
	}
	return value, nil
}

func decodeUniqueValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token := token.(type) {
	case json.Delim:
		switch token {
		case '{':
			result := map[string]any{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				name, ok := key.(string)
				if !ok {
					return nil, fmt.Errorf("JSON object key is invalid")
				}
				if _, exists := result[name]; exists {
					return nil, fmt.Errorf("duplicate JSON key %q", name)
				}
				value, err := decodeUniqueValue(decoder)
				if err != nil {
					return nil, err
				}
				result[name] = value
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return result, nil
		case '[':
			result := []any{}
			for decoder.More() {
				value, err := decodeUniqueValue(decoder)
				if err != nil {
					return nil, err
				}
				result = append(result, value)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return result, nil
		default:
			return nil, fmt.Errorf("JSON delimiter is invalid")
		}
	default:
		return token, nil
	}
}
