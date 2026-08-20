package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"tensors-router/internal/buildinfo"
	routerupdate "tensors-router/internal/update"
	"tensors-router/internal/vllm"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(arguments []string, input io.Reader, output io.Writer) error {
	if len(arguments) == 1 && (arguments[0] == "version" || arguments[0] == "--version" || arguments[0] == "-v") {
		_, _ = fmt.Fprintln(output, buildinfo.Current())
		return nil
	}
	if len(arguments) == 1 && arguments[0] == "bootstrap-info" {
		digest, err := vllm.EmbeddedUVBootstrap()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(output, "uv sha256:"+digest)
		return nil
	}
	if len(arguments) == 0 || arguments[0] == "help" || arguments[0] == "--help" || arguments[0] == "-h" {
		return usage(output)
	}
	if arguments[0] != "worker" {
		if arguments[0] == "profile-check" {
			return runProfileCheck(arguments[1:], output)
		}
		return fmt.Errorf("unknown command %q", arguments[0])
	}
	if !vllm.SupportedPlatform() {
		return fmt.Errorf("%s", vllm.UnsupportedReason())
	}
	configuration, err := parseWorkerConfig(arguments[1:])
	if err != nil {
		return err
	}
	// An operator-pinned manifest is authoritative: a missing or mismatched file is an
	// error, never a reason to install something else. The TUF tier falls back to the
	// embedded default, and only that, when the metadata chain verified but no runtime
	// manifest has been published; an explicit --allow-unverified-install extends that
	// chain one step further, to installing vLLM unpinned from PyPI. Nothing here lets
	// a TUF-signed or operator-pinned manifest itself select an unverified install --
	// only this flag can.
	var unverifiedSource vllm.ManifestSource
	if configuration.AllowUnverifiedInstall {
		unverifiedSource = vllm.UnverifiedManifestSource{VLLMVersion: configuration.UnverifiedVLLMVersion, PythonVersion: configuration.UnverifiedPythonVersion}
	}
	var manifestSource vllm.ManifestSource
	switch {
	case configuration.TUFRepositoryURL != "":
		manifestSource = vllm.FallbackManifestSource{
			Primary:  vllm.TUFManifestSource{RepositoryURL: configuration.TUFRepositoryURL, TrustedRootPath: configuration.TUFRootPath, TrustedRoot: routerupdate.TrustedRoot(), TargetPath: configuration.ManifestPath, CacheDir: configuration.DataDir + "/tuf"},
			Fallback: vllm.FallbackManifestSource{Primary: vllm.EmbeddedManifestSource{}, Fallback: unverifiedSource},
		}
	case configuration.ManifestPath != "":
		manifestSource = vllm.AuthorizedManifestFile{Path: configuration.ManifestPath, Authorization: vllm.ArtifactAuthorization{Length: configuration.ManifestSize, SHA256: configuration.ManifestSHA256}}
	default:
		manifestSource = unverifiedSource
	}
	manager, err := vllm.NewManager(vllm.ManagerOptions{
		DataDir:              configuration.DataDir,
		DefaultProfile:       configuration.DefaultProfile,
		ManifestSource:       manifestSource,
		Detector:             vllm.SystemDetector{},
		Downloader:           vllm.HTTPArtifactDownloader{},
		Installer:            vllm.UVEnvironmentInstaller{IndexURL: configuration.UnverifiedIndexURL, ExtraIndexURL: configuration.UnverifiedExtraIndexURL},
		SmokeTester:          vllm.CommandSmokeTester{},
		AllowTrustRemoteCode: configuration.AllowTrustRemoteCode,
		AllowExternalTools:   configuration.AllowExternalTools,
		AllowDynamicLoRA:     configuration.AllowDynamicLoRA,
	})
	if err != nil {
		return err
	}
	return vllm.ServeWorker(manager, input, output)
}

func parseWorkerConfig(arguments []string) (vllm.ClientConfig, error) {
	configuration := vllm.ClientConfig{DefaultProfile: "auto"}
	for index := 0; index < len(arguments); index++ {
		name := arguments[index]
		if index+1 >= len(arguments) {
			return vllm.ClientConfig{}, fmt.Errorf("%s requires a value", name)
		}
		value := strings.TrimSpace(arguments[index+1])
		index++
		switch name {
		case "--data-dir":
			configuration.DataDir = value
		case "--profile":
			configuration.DefaultProfile = value
		case "--manifest":
			configuration.ManifestPath = value
		case "--manifest-size":
			size, err := strconv.ParseInt(value, 10, 64)
			if err != nil || size <= 0 {
				return vllm.ClientConfig{}, fmt.Errorf("--manifest-size must be a positive integer")
			}
			configuration.ManifestSize = size
		case "--manifest-sha256":
			configuration.ManifestSHA256 = value
		case "--tuf-repository-url":
			configuration.TUFRepositoryURL = value
		case "--tuf-root":
			configuration.TUFRootPath = value
		case "--allow-trust-remote-code":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return vllm.ClientConfig{}, fmt.Errorf("%s must be true or false", name)
			}
			configuration.AllowTrustRemoteCode = parsed
		case "--allow-external-tools":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return vllm.ClientConfig{}, fmt.Errorf("%s must be true or false", name)
			}
			configuration.AllowExternalTools = parsed
		case "--allow-dynamic-lora":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return vllm.ClientConfig{}, fmt.Errorf("%s must be true or false", name)
			}
			configuration.AllowDynamicLoRA = parsed
		case "--allow-unverified-install":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return vllm.ClientConfig{}, fmt.Errorf("%s must be true or false", name)
			}
			configuration.AllowUnverifiedInstall = parsed
		case "--unverified-vllm-version":
			configuration.UnverifiedVLLMVersion = value
		case "--unverified-python-version":
			configuration.UnverifiedPythonVersion = value
		case "--unverified-index-url":
			configuration.UnverifiedIndexURL = value
		case "--unverified-extra-index-url":
			configuration.UnverifiedExtraIndexURL = value
		default:
			return vllm.ClientConfig{}, fmt.Errorf("unknown worker option %q", name)
		}
	}
	if configuration.DataDir == "" {
		return vllm.ClientConfig{}, fmt.Errorf("worker requires --data-dir")
	}
	switch {
	case configuration.TUFRepositoryURL != "":
		if configuration.ManifestPath == "" {
			return vllm.ClientConfig{}, fmt.Errorf("worker requires --manifest when --tuf-repository-url is set")
		}
	case configuration.ManifestPath != "":
		if configuration.ManifestSize <= 0 || configuration.ManifestSHA256 == "" {
			return vllm.ClientConfig{}, fmt.Errorf("worker requires --manifest-size and --manifest-sha256 when --manifest is set without --tuf-repository-url")
		}
	case !configuration.AllowUnverifiedInstall:
		return vllm.ClientConfig{}, fmt.Errorf("worker requires --tuf-repository-url, --manifest with --manifest-size and --manifest-sha256, or --allow-unverified-install")
	}
	return configuration, nil
}

func usage(output io.Writer) error {
	_, err := fmt.Fprintln(output, "Usage: tensor-router-vllm worker --data-dir PATH --profile PROFILE --manifest TARGET --tuf-repository-url URL --tuf-root PATH --allow-unverified-install BOOL")
	return err
}
