package vllm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Detector interface {
	Detect(context.Context) (Detection, error)
}

type ArtifactDownloader interface {
	Download(context.Context, Artifact, string, func(int64)) error
}

type EnvironmentInstaller interface {
	Install(context.Context, Profile, map[string]string, string, func(string) error) error
}

type SmokeTester interface {
	Test(context.Context, Profile, string) error
}

type ManagerOptions struct {
	DataDir              string
	DefaultProfile       string
	ManifestSource       ManifestSource
	Detector             Detector
	Downloader           ArtifactDownloader
	Installer            EnvironmentInstaller
	SmokeTester          SmokeTester
	RuntimeLauncher      RuntimeLauncher
	AllowTrustRemoteCode bool
	AllowExternalTools   bool
	AllowDynamicLoRA     bool
	// OCIRunAsImageUser lets an OCI runtime keep the image own user instead of the
	// host user. Needed for vendor images that install their interpreter under /root.
	OCIRunAsImageUser bool
	DisableRecovery   bool
}

type Manager struct {
	mu            sync.Mutex
	options       ManagerOptions
	dataDir       string
	job           InitializationJob
	worker        *initializationWorker
	active        activeEnvironment
	runtimes      map[RuntimeKind]*runtimeProcess
	closed        bool
	workers       sync.WaitGroup
	closeWait     time.Duration
	now           func() time.Time
	jobWriter     func(string, any, os.FileMode) error
	launchOptions LaunchOptions
}

type initializationWorker struct {
	jobID  string
	cancel context.CancelFunc
}

type activeEnvironment struct {
	ProfileID       string   `json:"profile_id"`
	VLLMVersion     string   `json:"vllm_version"`
	ManifestSHA256  string   `json:"manifest_sha256"`
	Path            string   `json:"path"`
	InstallMethod   string   `json:"install_method,omitempty"`
	OCIImage        string   `json:"oci_image,omitempty"`
	ContainerEngine string   `json:"container_engine,omitempty"`
	Devices         []string `json:"devices,omitempty"`
}

type environmentMarker struct {
	ProfileID       string    `json:"profile_id"`
	VLLMVersion     string    `json:"vllm_version"`
	ManifestSHA256  string    `json:"manifest_sha256"`
	CreatedAt       time.Time `json:"created_at"`
	InstallMethod   string    `json:"install_method,omitempty"`
	OCIImage        string    `json:"oci_image,omitempty"`
	ContainerEngine string    `json:"container_engine,omitempty"`
	Devices         []string  `json:"devices,omitempty"`
}

func NewManager(options ManagerOptions) (*Manager, error) {
	if strings.TrimSpace(options.DataDir) == "" {
		return nil, fmt.Errorf("vLLM data directory is required")
	}
	if options.ManifestSource == nil || options.Detector == nil || options.Downloader == nil || options.Installer == nil || options.SmokeTester == nil {
		return nil, fmt.Errorf("vLLM manifest, detector, downloader, installer, and smoke tester are required")
	}
	dataDir, err := filepath.Abs(options.DataDir)
	if err != nil {
		return nil, err
	}
	if err := ensurePrivateDirectory(dataDir); err != nil {
		return nil, err
	}
	for _, name := range []string{"state", "environments", "staging"} {
		if err := ensurePrivateDirectory(filepath.Join(dataDir, name)); err != nil {
			return nil, err
		}
	}
	manager := &Manager{
		options:   options,
		dataDir:   dataDir,
		runtimes:  map[RuntimeKind]*runtimeProcess{},
		closeWait: 10 * time.Second,
		now:       time.Now,
		jobWriter: writeJSONAtomic,
	}
	if manager.options.DefaultProfile == "" {
		manager.options.DefaultProfile = "auto"
	}
	if manager.options.RuntimeLauncher == nil {
		manager.options.RuntimeLauncher = ExecRuntimeLauncher{}
	}
	if err := manager.loadPersistentState(); err != nil {
		return nil, err
	}
	if !options.DisableRecovery && (manager.job.State == JobQueued || manager.job.State == JobRunning) {
		manager.resumeInitialization()
	}
	return manager, nil
}

func (manager *Manager) State(context.Context) State {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	state := State{LifecycleState: LifecycleNeedsInit}
	if manager.active.Path != "" {
		state.LifecycleState = LifecycleReady
		state.SelectedProfile = manager.active.ProfileID
		state.RuntimeVersion = manager.active.VLLMVersion
	}
	if manager.job.JobID == "" {
		return state
	}
	state.SelectedProfile = manager.job.SelectedProfile
	state.DetectedProfile = manager.job.DetectedProfile
	state.InitializationJobID = manager.job.JobID
	state.InitializationPhase = manager.job.Phase
	state.ManifestTrust = manager.job.ManifestTrust
	state.LaunchOptions = manager.launchOptions
	state.InitializationBytes = manager.job.CompletedBytes
	state.InitializationTotalBytes = manager.job.TotalBytes
	state.Error = manager.job.Error
	state.Retryable = manager.job.Retryable
	switch manager.job.State {
	case JobQueued, JobRunning:
		state.LifecycleState = LifecycleInitializing
	case JobFailed:
		state.LifecycleState = LifecycleFailed
	case JobCompleted:
		if manager.active.Path == "" {
			state.LifecycleState = LifecycleNeedsInit
		}
	}
	return state
}
