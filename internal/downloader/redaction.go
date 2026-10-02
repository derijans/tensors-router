package downloader

import (
	"regexp"
	"strings"
)

const redactedPlaceholder = "[redacted]"

var credentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bhf_[A-Za-z0-9]{8,}`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{8,}`),
}

var labelledCredentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(\bbearer\s+)[^\s"',;]+`),
	regexp.MustCompile(`(?i)(\b(?:token|access_token|api_key|apikey|password)\s*[=:]\s*)[^\s"',;&]+`),
}

func redactSensitive(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret = strings.TrimSpace(secret); secret != "" {
			value = strings.ReplaceAll(value, secret, redactedPlaceholder)
		}
	}
	for _, pattern := range credentialPatterns {
		value = pattern.ReplaceAllString(value, redactedPlaceholder)
	}
	for _, pattern := range labelledCredentialPatterns {
		value = pattern.ReplaceAllString(value, "${1}"+redactedPlaceholder)
	}
	return value
}
