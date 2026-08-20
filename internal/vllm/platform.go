package vllm

import (
	"fmt"
	"runtime"
)

func SupportedPlatform() bool {
	return supportedPlatform(runtime.GOOS, runtime.GOARCH)
}

// PlatformKey returns the "<os>-<arch>" key used for manifest targets and embedded
// defaults on the running host.
func PlatformKey() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

func UnsupportedReason() string {
	if SupportedPlatform() {
		return ""
	}
	return fmt.Sprintf("vLLM companion is unsupported on %s/%s", runtime.GOOS, runtime.GOARCH)
}

func supportedPlatform(operatingSystem string, architecture string) bool {
	return operatingSystem == "linux" && (architecture == "amd64" || architecture == "arm64") || operatingSystem == "darwin" && architecture == "arm64"
}
