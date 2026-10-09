package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"tensors-router/internal/atomicfile"
)

const routerMCPDirectory = ".router-mcp"

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
	directory := config.Directory
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
	return reconciler.reconcileLocked(filename, backend)
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
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".kcpps") || strings.HasPrefix(entry.Name(), routerMCPDirectory) {
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
	return reconciler.removeArtifacts(stem, filename)
}

func Validate(content []byte, fallbackBackend string) error {
	_, _, _, err := decodeServers(content, fallbackBackend)
	return err
}

func (reconciler *Reconciler) reconcileLocked(filename string, backend string) (Result, error) {
	target, stem, err := reconciler.configTarget(filename)
	if err != nil {
		return Result{}, err
	}
	if !reconciler.enabled {
		return Result{}, reconciler.removeArtifacts(stem, filename)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return Result{}, err
	}
	servers, enabled, effectiveBackend, err := decodeServers(content, backend)
	if err != nil {
		return Result{}, err
	}
	if !enabled {
		return Result{}, reconciler.removeArtifacts(stem, filename)
	}
	artifactDirectory, err := validatedArtifactDirectory(reconciler.directory, stem)
	if err != nil {
		return Result{}, err
	}
	serversPath := filepath.Join(artifactDirectory, "servers.json")
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
	if effectiveBackend == BackendKobold {
		overlayPath := filepath.Join(reconciler.configDir, routerMCPDirectory, filename)
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
	} else {
		if err := removeValidatedFile(filepath.Join(reconciler.configDir, routerMCPDirectory), filename); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

func (reconciler *Reconciler) removeArtifacts(stem string, filename string) error {
	if err := removeValidatedDirectory(reconciler.directory, stem); err != nil {
		return err
	}
	return removeValidatedFile(filepath.Join(reconciler.configDir, routerMCPDirectory), filename)
}

func (reconciler *Reconciler) configTarget(filename string) (string, string, error) {
	if filename == "" || filename != filepath.Base(filename) || !filepath.IsLocal(filename) || !strings.EqualFold(filepath.Ext(filename), ".kcpps") {
		return "", "", fmt.Errorf("config filename is invalid")
	}
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	if stem == "" || stem == "." || stem == ".." || stem != filepath.Base(stem) || !filepath.IsLocal(stem) {
		return "", "", fmt.Errorf("config filename is invalid")
	}
	return filepath.Join(reconciler.configDir, filename), stem, nil
}

func absoluteDirectory(value string) (string, error) {
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
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("MCP artifact path %q is not a private directory", path)
	}
	return os.Chmod(path, 0o700)
}

func removeValidatedDirectory(root string, name string) error {
	target, err := validatedArtifactDirectory(root, name)
	if err != nil {
		return err
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

func validatedArtifactDirectory(root string, name string) (string, error) {
	if name == "" || name == "." || name == ".." || name != filepath.Base(name) || !filepath.IsLocal(name) {
		return "", fmt.Errorf("artifact directory is invalid")
	}
	cleanRoot := filepath.Clean(root)
	target := filepath.Join(cleanRoot, name)
	if filepath.Dir(target) != cleanRoot {
		return "", fmt.Errorf("artifact directory is invalid")
	}
	return target, nil
}

func removeValidatedFile(root string, name string) error {
	if name == "" || name != filepath.Base(name) || !filepath.IsLocal(name) {
		return fmt.Errorf("artifact filename is invalid")
	}
	cleanRoot := filepath.Clean(root)
	rootInfo, err := os.Lstat(cleanRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return fmt.Errorf("artifact root is unsafe")
	}
	target := filepath.Join(cleanRoot, name)
	if filepath.Dir(target) != cleanRoot {
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

func generatedServers(servers []server) ([]byte, error) {
	sort.Slice(servers, func(left, right int) bool { return servers[left].Name < servers[right].Name })
	definitions := make(map[string]json.RawMessage, len(servers))
	for _, server := range servers {
		definitions[server.Name] = server.Definition
	}
	return json.Marshal(map[string]any{"mcpServers": definitions})
}
