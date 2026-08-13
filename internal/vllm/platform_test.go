package vllm

import "testing"

func TestSupportedPlatformMatrix(t *testing.T) {
	tests := []struct {
		operatingSystem string
		architecture    string
		supported       bool
	}{
		{operatingSystem: "linux", architecture: "amd64", supported: true},
		{operatingSystem: "linux", architecture: "arm64", supported: true},
		{operatingSystem: "darwin", architecture: "arm64", supported: true},
		{operatingSystem: "windows", architecture: "amd64", supported: false},
		{operatingSystem: "linux", architecture: "arm", supported: false},
		{operatingSystem: "darwin", architecture: "amd64", supported: false},
	}
	for _, test := range tests {
		if supportedPlatform(test.operatingSystem, test.architecture) != test.supported {
			t.Fatalf("unexpected support for %s/%s", test.operatingSystem, test.architecture)
		}
	}
}
