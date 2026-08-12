//go:build !windows

package vllm

func runtimeIdentityExpected() bool {
	return true
}
