package credential

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"
)

type TokenHash [sha256.Size]byte

func HashToken(token string) TokenHash {
	return sha256.Sum256([]byte(strings.TrimSpace(token)))
}

func MatchesAnyHash(token string, hashes []TokenHash) bool {
	presented := HashToken(token)
	matched := 0
	for _, hash := range hashes {
		matched |= subtle.ConstantTimeCompare(presented[:], hash[:])
	}
	return matched == 1
}

type KeyHashes struct {
	Inference []TokenHash
	Admin     []TokenHash
	Metrics   []TokenHash
	Cluster   []TokenHash
}
