package vllm

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"time"
)

type UVEnvironmentInstaller struct {
	Runner          CommandRunner
	Logs            io.Writer
	ContainerEngine string
	// IndexURL and ExtraIndexURL are only used by the "pypi" install method, most
	// commonly to reach a CUDA/ROCm-specific torch wheel index.
	IndexURL      string
	ExtraIndexURL string
}

func (installer UVEnvironmentInstaller) Install(ctx context.Context, profile Profile, artifacts map[string]string, environmentPath string, phase func(string) error) error {
	if profile.InstallMethod == "pypi" {
		return installer.installFromPyPI(ctx, profile, environmentPath, phase)
	}
	if err := stageSmokeModel(ctx, profile, artifacts, environmentPath, phase); err != nil {
		return err
	}
	if profile.InstallMethod == "oci" {
		return installer.installOCI(ctx, profile, artifacts, environmentPath, phase)
	}
	pythonArtifactPath, err := artifactPathByRole(profile, artifacts, "python")
	if err != nil {
		return err
	}
	pythonArtifact, found := artifactByRole(profile, "python")
	if !found {
		return fmt.Errorf("vLLM profile requires exactly one python artifact")
	}
	bootstrapDirectory := filepath.Join(environmentPath, "bootstrap")
	if err := ensurePrivateDirectory(bootstrapDirectory); err != nil {
		return err
	}
	uvPath := filepath.Join(bootstrapDirectory, "uv")
	if runtime.GOOS == "windows" {
		uvPath += ".exe"
	}
	embedded, err := stageEmbeddedUV(uvPath)
	if err != nil {
		return fmt.Errorf("stage embedded uv bootstrap: %w", err)
	}
	if !embedded {
		uvArtifactPath, artifactError := artifactPathByRole(profile, artifacts, "uv")
		if artifactError != nil {
			return fmt.Errorf("companion has no embedded uv bootstrap and profile fallback is unavailable: %w", artifactError)
		}
		if err := copyRegularFile(ctx, uvArtifactPath, uvPath, 0o700); err != nil {
			return fmt.Errorf("stage authorized uv bootstrap fallback: %w", err)
		}
	}
	pythonRuntimeDirectory := filepath.Join(bootstrapDirectory, "python-runtime")
	if err := ensurePrivateDirectory(pythonRuntimeDirectory); err != nil {
		return err
	}
	if err := extractAuthorizedArchive(ctx, pythonArtifactPath, pythonRuntimeDirectory, pythonArtifact, "Python runtime"); err != nil {
		return fmt.Errorf("stage isolated Python runtime: %w", err)
	}
	pythonPath := filepath.Join(pythonRuntimeDirectory, filepath.FromSlash(pythonArtifact.ExecutablePath))
	if err := validatePortableInterpreterPath(pythonRuntimeDirectory, pythonPath); err != nil {
		return err
	}
	if err := makeExecutable(uvPath); err != nil {
		return err
	}
	runner := installer.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}
	logs := installer.Logs
	if logs == nil {
		logs = io.Discard
	}
	environment := isolatedInstallEnvironment(environmentPath)
	versionCheck := "import sys; assert '.'.join(map(str,sys.version_info[:3])) == " + strconv.Quote(profile.PythonVersion)
	if err := runner.Run(ctx, pythonPath, []string{"-I", "-c", versionCheck}, environment, environmentPath, logs); err != nil {
		return fmt.Errorf("validate isolated Python runtime version: %w", err)
	}
	if err := phase("creating_environment"); err != nil {
		return err
	}
	if err := runner.Run(ctx, uvPath, []string{"venv", "--python", pythonPath, "--no-project", environmentPath}, environment, environmentPath, logs); err != nil {
		return fmt.Errorf("create isolated Python environment: %w", err)
	}
	if err := phase("installing_packages"); err != nil {
		return err
	}
	packageArtifacts := artifactPathsByRole(profile, artifacts, "vllm", "plugin", "dependency", "source")
	if len(packageArtifacts) == 0 {
		return fmt.Errorf("vLLM profile contains no package artifacts")
	}
	arguments := []string{"pip", "install", "--python", environmentPythonPath(environmentPath), "--offline", "--no-index", "--no-deps"}
	for _, directory := range artifactDirectories(packageArtifacts) {
		arguments = append(arguments, "--find-links", directory)
	}
	arguments = append(arguments, packageArtifacts...)
	if err := runner.Run(ctx, uvPath, arguments, environment, environmentPath, logs); err != nil {
		return fmt.Errorf("install authorized vLLM packages: %w", err)
	}
	return nil
}

func (installer UVEnvironmentInstaller) installOCI(ctx context.Context, profile Profile, artifacts map[string]string, environmentPath string, phase func(string) error) error {
	imagePath, err := artifactPathByRole(profile, artifacts, "oci")
	if err != nil {
		return err
	}
	if err := phase("staging_oci_image"); err != nil {
		return err
	}
	destination := filepath.Join(environmentPath, "runtime.oci")
	if err := copyRegularFile(ctx, imagePath, destination, 0o600); err != nil {
		return err
	}
	enginePath, engineName, err := findContainerEngine(installer.ContainerEngine)
	if err != nil {
		return err
	}
	if err := phase("importing_oci_image"); err != nil {
		return err
	}
	runner := installer.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}
	logs := installer.Logs
	if logs == nil {
		logs = io.Discard
	}
	if err := runner.Run(ctx, enginePath, []string{"load", "--input", destination}, containerEngineEnvironment(environmentPath), environmentPath, logs); err != nil {
		return fmt.Errorf("import authorized OCI image: %w", err)
	}
	if err := runner.Run(ctx, enginePath, []string{"image", "inspect", profile.OCIImage}, containerEngineEnvironment(environmentPath), environmentPath, logs); err != nil {
		return fmt.Errorf("verify authorized OCI image id: %w", err)
	}
	return writeJSONAtomic(filepath.Join(environmentPath, ociMetadataFilename), ociRuntimeMetadata{Engine: engineName, Image: profile.OCIImage}, 0o600)
}

// installFromPyPI installs vLLM directly from PyPI with uv resolving dependencies
// normally. Unlike every other install path here, nothing downloaded in this method is
// digest-verified: there is no artifact list to check against, because there is no
// manifest authorizing specific bytes. It is only reachable through
// UnverifiedManifestSource, which an operator must explicitly enable.
func (installer UVEnvironmentInstaller) installFromPyPI(ctx context.Context, profile Profile, environmentPath string, phase func(string) error) error {
	bootstrapDirectory := filepath.Join(environmentPath, "bootstrap")
	if err := ensurePrivateDirectory(bootstrapDirectory); err != nil {
		return err
	}
	uvPath := filepath.Join(bootstrapDirectory, "uv")
	if runtime.GOOS == "windows" {
		uvPath += ".exe"
	}
	embedded, err := stageEmbeddedUV(uvPath)
	if err != nil {
		return fmt.Errorf("stage embedded uv bootstrap: %w", err)
	}
	if !embedded {
		return fmt.Errorf("companion has no embedded uv bootstrap; unverified installs require a build with the embedded uv bootstrap")
	}
	if err := makeExecutable(uvPath); err != nil {
		return err
	}
	runner := installer.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}
	logs := installer.Logs
	if logs == nil {
		logs = io.Discard
	}
	environment := networkInstallEnvironment(environmentPath)
	if err := phase("creating_environment"); err != nil {
		return err
	}
	if err := runner.Run(ctx, uvPath, []string{"venv", "--python", profile.PythonVersion, "--no-project", environmentPath}, environment, environmentPath, logs); err != nil {
		return fmt.Errorf("create isolated Python environment: %w", err)
	}
	if err := phase("installing_packages"); err != nil {
		return err
	}
	packageSpec := "vllm"
	if profile.VLLMVersion != "" {
		packageSpec = "vllm==" + profile.VLLMVersion
	}
	arguments := []string{"pip", "install", "--python", environmentPythonPath(environmentPath)}
	if installer.IndexURL != "" {
		arguments = append(arguments, "--index-url", installer.IndexURL)
	}
	if installer.ExtraIndexURL != "" {
		arguments = append(arguments, "--extra-index-url", installer.ExtraIndexURL)
	}
	arguments = append(arguments, packageSpec)
	if err := runner.Run(ctx, uvPath, arguments, environment, environmentPath, logs); err != nil {
		return fmt.Errorf("install unpinned vLLM package from PyPI: %w", err)
	}
	return nil
}

type CommandSmokeTester struct {
	Runner          CommandRunner
	Launcher        RuntimeLauncher
	Logs            io.Writer
	ContainerEngine string
	ServingTimeout  time.Duration
}

func (tester CommandSmokeTester) Test(ctx context.Context, profile Profile, environmentPath string) error {
	if profile.InstallMethod == "oci" {
		return tester.testOCI(ctx, profile, environmentPath)
	}
	runner := tester.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}
	logs := tester.Logs
	if logs == nil {
		logs = io.Discard
	}
	pythonPath := environmentPythonPath(environmentPath)
	environment := isolatedRuntimeEnvironment(environmentPath, "")
	if profile.InstallMethod == "pypi" {
		// No signed smoke model exists for an unverified install, so this checks
		// only that the unpinned package imports - not that it can actually serve.
		versionCheck := "import sys,vllm; assert '.'.join(map(str,sys.version_info[:3])) == " + strconv.Quote(profile.PythonVersion)
		if profile.VLLMVersion != "" {
			versionCheck += "; assert vllm.__version__ == " + strconv.Quote(profile.VLLMVersion)
		}
		if err := runner.Run(ctx, pythonPath, []string{"-I", "-c", versionCheck}, environment, environmentPath, logs); err != nil {
			return fmt.Errorf("import unverified vLLM install: %w", err)
		}
		return nil
	}
	versionCheck := "import sys,vllm; assert vllm.__version__ == " + strconv.Quote(profile.VLLMVersion) + "; assert '.'.join(map(str,sys.version_info[:3])) == " + strconv.Quote(profile.PythonVersion)
	if err := runner.Run(ctx, pythonPath, []string{"-I", "-c", versionCheck}, environment, environmentPath, logs); err != nil {
		return fmt.Errorf("import vLLM: %w", err)
	}
	pluginNames := make([]string, 0, len(profile.PluginVersions))
	for name := range profile.PluginVersions {
		pluginNames = append(pluginNames, name)
	}
	sort.Strings(pluginNames)
	for _, name := range pluginNames {
		pluginCheck := "import importlib.metadata as m; assert m.version(" + strconv.Quote(name) + ") == " + strconv.Quote(profile.PluginVersions[name])
		if err := runner.Run(ctx, pythonPath, []string{"-I", "-c", pluginCheck}, environment, environmentPath, logs); err != nil {
			return fmt.Errorf("verify vLLM plugin %q: %w", name, err)
		}
	}
	return tester.testNativeServing(ctx, pythonPath, environmentPath, environment, logs)
}

func (tester CommandSmokeTester) testOCI(ctx context.Context, profile Profile, environmentPath string) error {
	metadata, err := readOCIRuntimeMetadata(environmentPath)
	if err != nil {
		return err
	}
	preferredEngine := tester.ContainerEngine
	if preferredEngine == "" {
		preferredEngine = metadata.Engine
	}
	enginePath, engineName, err := findContainerEngine(preferredEngine)
	if err != nil {
		return err
	}
	if engineName != metadata.Engine || metadata.Image != profile.OCIImage {
		return fmt.Errorf("staged OCI runtime metadata does not match profile")
	}
	runner := tester.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}
	logs := tester.Logs
	if logs == nil {
		logs = io.Discard
	}
	versionCheck := "import sys,vllm; assert vllm.__version__ == " + strconv.Quote(profile.VLLMVersion) + "; assert '.'.join(map(str,sys.version_info[:3])) == " + strconv.Quote(profile.PythonVersion)
	checks := [][]string{{"-I", "-c", versionCheck}}
	pluginNames := make([]string, 0, len(profile.PluginVersions))
	for name := range profile.PluginVersions {
		pluginNames = append(pluginNames, name)
	}
	sort.Strings(pluginNames)
	for _, name := range pluginNames {
		pluginCheck := "import importlib.metadata as m; assert m.version(" + strconv.Quote(name) + ") == " + strconv.Quote(profile.PluginVersions[name])
		checks = append(checks, []string{"-I", "-c", pluginCheck})
	}
	for _, check := range checks {
		arguments := ociCommandArguments(engineName, profile, nil, nil, check, false)
		if err := runner.Run(ctx, enginePath, arguments, containerEngineEnvironment(environmentPath), environmentPath, logs); err != nil {
			return fmt.Errorf("smoke test authorized OCI runtime: %w", err)
		}
	}
	return tester.testOCIServing(ctx, profile, environmentPath, enginePath, engineName, logs)
}
