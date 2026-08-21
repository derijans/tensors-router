package vllm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
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
	// DataDir is the companion's data directory. The "pypi" install method needs a
	// location for uv-managed interpreters that outlives the staging directory,
	// because a staged environment is moved when it is promoted.
	DataDir string
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
	// --allow-existing: the staged environment already holds the smoke model and the
	// bootstrap directory, and uv refuses to create a virtual environment in a
	// non-empty directory without it. It preserves what is already there.
	if err := runner.Run(ctx, uvPath, []string{"venv", "--python", pythonPath, "--no-project", "--allow-existing", environmentPath}, environment, environmentPath, logs); err != nil {
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
	// uv reports why it failed on its own output. Capture it alongside whatever the
	// caller is logging so a failure names its cause instead of only an exit status.
	captured := newBoundedLog(maximumRuntimeLogBytes)
	output := io.MultiWriter(logs, captured)
	failure := func(action string, err error) error {
		if details := captured.String(); details != "" {
			return fmt.Errorf("%s: %w: %s", action, err, details)
		}
		return fmt.Errorf("%s: %w", action, err)
	}
	environment := networkInstallEnvironment(environmentPath)
	if err := phase("creating_environment"); err != nil {
		return err
	}
	// --allow-existing: the bootstrap directory staged above already lives inside the
	// environment, and uv refuses a non-empty target without it.
	if err := runner.Run(ctx, uvPath, []string{"venv", "--python", profile.PythonVersion, "--no-project", "--allow-existing", environmentPath}, environment, environmentPath, output); err != nil {
		return failure("create isolated Python environment", err)
	}
	if err := phase("installing_packages"); err != nil {
		return err
	}
	packageSpec := "vllm"
	if profile.VLLMVersion != "" {
		packageSpec = "vllm==" + profile.VLLMVersion
	}
	// These images ship no compiler on purpose, so building vLLM from an sdist can
	// never succeed. Without this the resolver silently backtracks to an ancient
	// release that has no wheel for the platform, downloads gigabytes of
	// dependencies, and only then fails inside a source build. Requiring a wheel
	// fails fast and honestly instead.
	arguments := []string{"pip", "install", "--python", environmentPythonPath(environmentPath), "--only-binary", "vllm"}
	if installer.IndexURL != "" {
		arguments = append(arguments, "--index-url", installer.IndexURL)
	}
	if installer.ExtraIndexURL != "" {
		// Deliberately left on uv's default index strategy. Widening it to
		// unsafe-best-match makes otherwise-unsatisfiable resolutions succeed, but it
		// does so by letting a newer wheel on PyPI outrank the accelerator index the
		// operator named - installing a CUDA stack on a ROCm host, or a newer CUDA
		// variant than the driver supports. Failing to resolve is recoverable;
		// silently installing the wrong accelerator build is not.
		arguments = append(arguments, "--extra-index-url", installer.ExtraIndexURL)
	}
	arguments = append(arguments, packageSpec)
	if err := runner.Run(ctx, uvPath, arguments, environment, environmentPath, output); err != nil {
		return failure("install unpinned vLLM package from PyPI", err)
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
		// Capture the interpreter's own output: an import failure here is usually a
		// missing accelerator library, which the traceback names and an exit status
		// does not.
		captured := newBoundedLog(maximumRuntimeLogBytes)
		// unverified_python_version is an interpreter *request* such as "3.12", which
		// uv resolves to a specific patch release like 3.12.3. Match it as a version
		// prefix rather than demanding exact equality the way pinned profiles do.
		versionCheck := "import sys,vllm; requested = " + strconv.Quote(profile.PythonVersion) +
			"; actual = '.'.join(map(str,sys.version_info[:3]))" +
			"; assert actual == requested or actual.startswith(requested + '.'), 'python ' + actual + ' does not match requested ' + requested"
		if profile.VLLMVersion != "" {
			versionCheck += "; assert vllm.__version__ == " + strconv.Quote(profile.VLLMVersion) + ", 'vllm ' + vllm.__version__ + ' does not match pinned ' + " + strconv.Quote(profile.VLLMVersion)
		}
		if err := runner.Run(ctx, pythonPath, []string{"-I", "-c", versionCheck}, environment, environmentPath, io.MultiWriter(logs, captured)); err != nil {
			if details := captured.String(); details != "" {
				return fmt.Errorf("import unverified vLLM install: %w: %s", err, details)
			}
			return fmt.Errorf("import unverified vLLM install: %w", err)
		}
		if err := tester.checkUnverifiedAccelerator(ctx, runner, profile, pythonPath, environment, environmentPath, logs); err != nil {
			return err
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

// checkUnverifiedAccelerator rejects an install whose torch build does not match the
// accelerator this host actually has. PyPI publishes only CUDA-built vLLM wheels, so on
// an AMD host the resolver happily produces a complete, importable, and entirely
// unusable CUDA stack. Catching it here costs one interpreter call and turns an opaque
// "runtime exited" at first inference into an explanation.
func (tester CommandSmokeTester) checkUnverifiedAccelerator(ctx context.Context, runner CommandRunner, profile Profile, pythonPath string, environment []string, environmentPath string, logs io.Writer) error {
	expectsROCm := containsFold(profile.Devices, "rocm")
	expectsCUDA := containsFold(profile.Devices, "cuda")
	if !expectsROCm && !expectsCUDA {
		return nil
	}
	captured := newBoundedLog(maximumRuntimeLogBytes)
	probe := "import torch,json; print(json.dumps({'version': torch.__version__, 'hip': torch.version.hip, 'available': torch.cuda.is_available()}))"
	if err := runner.Run(ctx, pythonPath, []string{"-I", "-c", probe}, environment, environmentPath, io.MultiWriter(logs, captured)); err != nil {
		return fmt.Errorf("inspect installed torch build: %w: %s", err, captured.String())
	}
	var report struct {
		Version   string `json:"version"`
		HIP       string `json:"hip"`
		Available bool   `json:"available"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(captured.String())), &report); err != nil {
		return fmt.Errorf("inspect installed torch build: %w", err)
	}
	if expectsROCm && report.HIP == "" {
		return fmt.Errorf("this host reports a ROCm accelerator but the resolved torch %q is a CUDA build; PyPI publishes only CUDA vLLM wheels, so an unverified install cannot target ROCm - use an OCI profile built for ROCm instead", report.Version)
	}
	if expectsCUDA && report.HIP != "" {
		return fmt.Errorf("this host reports a CUDA accelerator but the resolved torch %q is a ROCm build", report.Version)
	}
	if !report.Available {
		return fmt.Errorf("the resolved torch %q reports no usable accelerator on a host that has one; the installed build does not match this hardware", report.Version)
	}
	return nil
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
