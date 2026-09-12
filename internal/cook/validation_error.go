package cook

import (
	"errors"
	"strings"
)

type ValidationError struct {
	Issues []ValidationIssue
}

func (err ValidationError) Error() string {
	messages := make([]string, 0, len(err.Issues))
	for _, issue := range err.Issues {
		if issue.Severity == "error" {
			messages = append(messages, issue.Message)
		}
	}
	if len(messages) == 0 {
		return "validation failed"
	}
	return strings.Join(messages, "; ")
}

func IssuesFromError(err error) ([]ValidationIssue, bool) {
	var validation ValidationError
	if errors.As(err, &validation) {
		return validation.Issues, true
	}
	return nil, false
}
