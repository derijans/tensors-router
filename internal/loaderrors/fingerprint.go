package loaderrors

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
)

var volatileNumber = regexp.MustCompile(`\b\d[\d.:_-]*\b`)

func fingerprint(input RecordInput, message string) string {
	normalized := volatileNumber.ReplaceAllString(strings.ToLower(message), "N")
	digest := sha256.Sum256([]byte(strings.Join([]string{
		string(input.Phase),
		strings.TrimSpace(input.Source),
		strings.TrimSpace(input.NodeID),
		strings.TrimSpace(input.ConfigName),
		strings.TrimSpace(input.Backend),
		normalized,
	}, "\x1f")))
	return hex.EncodeToString(digest[:])
}

func redact(value string, secrets []string) string {
	for _, secret := range sortedByLengthDesc(secrets) {
		if secret = strings.TrimSpace(secret); secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	fields := strings.Fields(value)
	for index, field := range fields {
		lower := strings.ToLower(field)
		if strings.HasPrefix(field, "hf_") || strings.HasPrefix(field, "sk-") || strings.Contains(lower, "authorization:") || strings.Contains(lower, "bearer") {
			fields[index] = "[REDACTED]"
		}
	}
	return strings.Join(fields, " ")
}

func sortedByLengthDesc(values []string) []string {
	ordered := append([]string(nil), values...)
	sort.Slice(ordered, func(left, right int) bool { return len(ordered[left]) > len(ordered[right]) })
	return ordered
}

func clampOutput(value string, limit int) (string, bool) {
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	return value[len(value)-limit:], true
}
