package backenddiagnostic

import (
	"bytes"
	"errors"
	"strings"
	"sync"
)

const MaxOutputBytes = 256 * 1024

type Diagnostic struct {
	Output    string `json:"output,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	NodeID    string `json:"node_id,omitempty"`
	Backend   string `json:"backend,omitempty"`
	ExitError string `json:"exit_error,omitempty"`
}

type Capture struct {
	mu        sync.Mutex
	output    []byte
	truncated bool
	active    bool
	secrets   []string
	exitError string
}

func NewCapture(secrets ...string) *Capture {
	return &Capture{secrets: nonEmpty(secrets)}
}

func (capture *Capture) Begin() {
	capture.mu.Lock()
	capture.output = nil
	capture.truncated = false
	capture.exitError = ""
	capture.active = true
	capture.mu.Unlock()
}

func (capture *Capture) End(success bool) Diagnostic {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	diagnostic := capture.snapshotLocked()
	capture.active = false
	if success {
		capture.output = nil
		capture.truncated = false
		capture.exitError = ""
	}
	return diagnostic
}

func (capture *Capture) Write(content []byte) (int, error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if !capture.active || len(content) == 0 {
		return len(content), nil
	}
	capture.output = append(capture.output, content...)
	if len(capture.output) > MaxOutputBytes {
		capture.output = append([]byte(nil), capture.output[len(capture.output)-MaxOutputBytes:]...)
		capture.truncated = true
	}
	return len(content), nil
}

func (capture *Capture) RecordExit(err error) {
	capture.mu.Lock()
	if capture.active {
		if err == nil {
			capture.exitError = "exit status 0"
		} else {
			capture.exitError = err.Error()
		}
	}
	capture.mu.Unlock()
}

func (capture *Capture) Snapshot() Diagnostic {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.snapshotLocked()
}

func (capture *Capture) snapshotLocked() Diagnostic {
	output := strings.ToValidUTF8(string(capture.output), "?")
	output = stripControlSequences(output)
	for _, secret := range capture.secrets {
		output = strings.ReplaceAll(output, secret, "[REDACTED]")
	}
	return Diagnostic{Output: output, Truncated: capture.truncated, ExitError: capture.exitError}
}

func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func stripControlSequences(value string) string {
	var result bytes.Buffer
	for index := 0; index < len(value); index++ {
		current := value[index]
		if current == 0x1b {
			if index+1 < len(value) && value[index+1] == '[' {
				index += 2
				for index < len(value) && (value[index] < 0x40 || value[index] > 0x7e) {
					index++
				}
				continue
			}
			if index+1 < len(value) && value[index+1] == ']' {
				index += 2
				for index < len(value) {
					if value[index] == 0x07 {
						break
					}
					if value[index] == 0x1b && index+1 < len(value) && value[index+1] == '\\' {
						index++
						break
					}
					index++
				}
				continue
			}
			if index+1 < len(value) {
				index++
			}
			continue
		}
		if current < 0x20 && current != '\n' && current != '\r' && current != '\t' {
			continue
		}
		result.WriteByte(current)
	}
	return result.String()
}

type Failure struct {
	Err        error
	Diagnostic Diagnostic
}

func (failure *Failure) Error() string { return failure.Err.Error() }

func (failure *Failure) Unwrap() error { return failure.Err }

func WithDiagnostic(err error, diagnostic Diagnostic) error {
	if err == nil {
		return nil
	}
	if diagnostic.Output == "" && diagnostic.ExitError == "" && diagnostic.NodeID == "" && diagnostic.Backend == "" {
		return err
	}
	var existing *Failure
	if errors.As(err, &existing) {
		return err
	}
	return &Failure{Err: err, Diagnostic: diagnostic}
}

func FromError(err error) (Diagnostic, bool) {
	var failure *Failure
	if !errors.As(err, &failure) {
		return Diagnostic{}, false
	}
	return failure.Diagnostic, true
}
