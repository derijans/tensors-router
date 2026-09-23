package credential

import (
	"fmt"
	"strings"
)

const RecommendedMinimumLength = 16

var placeholderValues = map[string]bool{
	"change-me":  true,
	"changeme":   true,
	"replace-me": true,
	"replace_me": true,
	"secret":     true,
	"password":   true,
	"token":      true,
	"admin":      true,
	"test":       true,
}

var placeholderPrefixes = []string{"replace-with", "your-", "example"}

func RejectPlaceholder(name string, value string) error {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if placeholderValues[normalized] {
		return fmt.Errorf("%s contains a known placeholder", name)
	}
	for _, prefix := range placeholderPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return fmt.Errorf("%s contains a known placeholder", name)
		}
	}
	return nil
}

func ShortCredentialWarning(name string, value string) (string, bool) {
	length := len(strings.TrimSpace(value))
	if length == 0 || length >= RecommendedMinimumLength {
		return "", false
	}
	return fmt.Sprintf("%s is shorter than %d characters; use a longer random value", name, RecommendedMinimumLength), true
}
