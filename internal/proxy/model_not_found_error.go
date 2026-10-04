package proxy

import "fmt"

type modelNotFoundError struct {
	modelID string
}

func (err modelNotFoundError) Error() string {
	return fmt.Sprintf("model %q was not found", err.modelID)
}
