package auth

import (
	"strings"

	"tensors-router/internal/credential"
)

type KeySource interface {
	KeyHashes() credential.KeyHashes
}

type staticKeys credential.KeyHashes

func (keys staticKeys) KeyHashes() credential.KeyHashes {
	return credential.KeyHashes(keys)
}

func staticKeySource(config PolicyConfig) staticKeys {
	return staticKeys{
		Inference: hashedKeys(config.InferenceKeys),
		Admin:     hashedKeys(config.AdminKeys),
		Metrics:   hashedKeys(config.MetricsKeys),
		Cluster:   hashedKeys([]string{config.ClusterToken}),
	}
}

func hashedKeys(values []string) []credential.TokenHash {
	hashes := make([]credential.TokenHash, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			hashes = append(hashes, credential.HashToken(value))
		}
	}
	return hashes
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token, token != ""
}

func allowedBearer(header string, hashes []credential.TokenHash) bool {
	token, ok := bearerToken(header)
	return ok && credential.MatchesAnyHash(token, hashes)
}
