package vllm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	tufmetadata "github.com/theupdateframework/go-tuf/v2/metadata"
	tufconfig "github.com/theupdateframework/go-tuf/v2/metadata/config"
	"github.com/theupdateframework/go-tuf/v2/metadata/updater"
)

type TUFManifestSource struct {
	RepositoryURL   string
	TrustedRootPath string
	TrustedRoot     []byte
	TargetPath      string
	CacheDir        string
	HTTPClient      *http.Client
}

func (source TUFManifestSource) Load(ctx context.Context) (Manifest, string, error) {
	repositoryURL, targetPath, root, err := source.validate()
	if err != nil {
		return Manifest{}, "", err
	}
	cacheDir, err := filepath.Abs(source.CacheDir)
	if err != nil {
		return Manifest{}, "", err
	}
	if err := ensurePrivateDirectory(cacheDir); err != nil {
		return Manifest{}, "", err
	}
	metadataDir := filepath.Join(cacheDir, "metadata")
	targetsDir := filepath.Join(cacheDir, "targets")
	if err := ensurePrivateDirectory(metadataDir); err != nil {
		return Manifest{}, "", err
	}
	if err := ensurePrivateDirectory(targetsDir); err != nil {
		return Manifest{}, "", err
	}
	if cachedRoot, readError := os.ReadFile(filepath.Join(metadataDir, "root.json")); readError == nil {
		root = cachedRoot
	} else if !os.IsNotExist(readError) {
		return Manifest{}, "", readError
	}
	configuration, err := tufconfig.New(repositoryURL, root)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("initialize vLLM TUF configuration: %w", err)
	}
	configuration.LocalMetadataDir = metadataDir
	configuration.LocalTargetsDir = targetsDir
	configuration.RemoteTargetsURL = strings.TrimSuffix(repositoryURL, "/metadata") + "/targets"
	configuration.Fetcher = tufContextFetcher{ctx: ctx, client: source.client()}
	client, err := updater.New(configuration)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("initialize vLLM TUF updater: %w", err)
	}
	if err := client.Refresh(); err != nil {
		return Manifest{}, "", fmt.Errorf("refresh vLLM trusted metadata: %w", err)
	}
	targetInfo, err := client.GetTargetInfo(targetPath)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("resolve trusted vLLM manifest %q: %w", targetPath, err)
	}
	_, content, err := client.DownloadTarget(targetInfo, filepath.Join(targetsDir, url.PathEscape(targetPath)), "")
	if err != nil {
		return Manifest{}, "", fmt.Errorf("download trusted vLLM manifest %q: %w", targetPath, err)
	}
	digest, exists := targetInfo.Hashes["sha256"]
	if !exists {
		return Manifest{}, "", fmt.Errorf("trusted vLLM manifest lacks SHA-256 metadata")
	}
	return ParseAuthorizedManifest(content, ArtifactAuthorization{Length: targetInfo.Length, SHA256: digest.String()})
}

func (source TUFManifestSource) validate() (string, string, []byte, error) {
	repositoryURL := strings.TrimRight(strings.TrimSpace(source.RepositoryURL), "/")
	parsed, err := url.Parse(repositoryURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", nil, fmt.Errorf("vLLM TUF repository must be an absolute credential-free HTTPS URL")
	}
	targetPath := strings.TrimSpace(source.TargetPath)
	if targetPath == "" || targetPath == "." || path.IsAbs(targetPath) || path.Clean(targetPath) != targetPath || strings.HasPrefix(targetPath, "../") || strings.ContainsAny(targetPath, "\\\x00\r\n") {
		return "", "", nil, fmt.Errorf("vLLM TUF target path is invalid")
	}
	root := append([]byte{}, source.TrustedRoot...)
	if strings.TrimSpace(source.TrustedRootPath) != "" {
		root, err = readBoundedRegularFile(source.TrustedRootPath, maximumManifestBytes)
		if err != nil {
			return "", "", nil, fmt.Errorf("read vLLM TUF trusted root: %w", err)
		}
	}
	if len(root) == 0 {
		return "", "", nil, fmt.Errorf("vLLM TUF trusted root is required")
	}
	if strings.TrimSpace(source.CacheDir) == "" {
		return "", "", nil, fmt.Errorf("vLLM TUF cache directory is required")
	}
	return repositoryURL, targetPath, root, nil
}

func (source TUFManifestSource) client() *http.Client {
	if source.HTTPClient != nil {
		return source.HTTPClient
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

type tufContextFetcher struct {
	ctx    context.Context
	client *http.Client
}

func (fetcher tufContextFetcher) DownloadFile(rawURL string, maximumLength int64, timeout time.Duration) ([]byte, error) {
	requestContext := fetcher.ctx
	cancel := func() {}
	if timeout > 0 {
		requestContext, cancel = context.WithTimeout(requestContext, timeout)
	}
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := fetcher.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, &tufmetadata.ErrDownloadHTTP{StatusCode: response.StatusCode, URL: rawURL}
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maximumLength+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maximumLength {
		return nil, fmt.Errorf("trusted vLLM download exceeds %d bytes", maximumLength)
	}
	return content, nil
}
