package proxy

import "testing"

func TestNodeInferencePathAcceptsSupportedRoutesAndRejectsExternalPaths(t *testing.T) {
	for _, requestPath := range []string{
		"/router/v1/node/inference/v1/chat/completions",
		"/router/v1/node/inference/v1/images/generations",
		"/router/v1/node/inference/v1/audio/speech",
		"/router/v1/node/inference/musicui/",
	} {
		if _, ok := nodeInferencePath(requestPath); !ok {
			t.Fatalf("supported inference route %q was rejected", requestPath)
		}
	}

	for _, requestPath := range []string{
		"/router/v1/node/inference//attacker.example/v1/chat/completions",
		"/router/v1/node/inference/\\attacker.example/v1/chat/completions",
		"/router/v1/node/inference/unknown",
	} {
		if _, ok := nodeInferencePath(requestPath); ok {
			t.Fatalf("unsafe inference route %q was accepted", requestPath)
		}
	}
}
