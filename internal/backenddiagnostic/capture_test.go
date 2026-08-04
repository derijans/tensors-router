package backenddiagnostic

import (
	"errors"
	"strings"
	"testing"
)

func TestCaptureBoundsSanitizesAndRedactsOutput(t *testing.T) {
	capture := NewCapture("router-secret")
	capture.Begin()
	_, _ = capture.Write([]byte(strings.Repeat("x", MaxOutputBytes+12)))
	_, _ = capture.Write([]byte("\x1b[31mrouter-secret\x1b[0m\x00\xff\n"))
	capture.RecordExit(errors.New("exit status 2"))

	diagnostic := capture.End(false)
	if len(diagnostic.Output) > MaxOutputBytes {
		t.Fatalf("diagnostic output exceeds cap: %d", len(diagnostic.Output))
	}
	if !diagnostic.Truncated {
		t.Fatal("expected truncated diagnostic")
	}
	if strings.Contains(diagnostic.Output, "router-secret") || strings.Contains(diagnostic.Output, "\x1b") || strings.Contains(diagnostic.Output, "\x00") {
		t.Fatalf("diagnostic was not sanitized: %q", diagnostic.Output[:min(80, len(diagnostic.Output))])
	}
	if diagnostic.ExitError != "exit status 2" {
		t.Fatalf("unexpected exit error %q", diagnostic.ExitError)
	}
}

func TestCaptureSuccessDiscardsPreviousLoad(t *testing.T) {
	capture := NewCapture()
	capture.Begin()
	_, _ = capture.Write([]byte("previous failure"))
	capture.End(true)
	capture.Begin()
	if diagnostic := capture.End(false); diagnostic.Output != "" || diagnostic.ExitError != "" {
		t.Fatalf("expected reset capture, got %#v", diagnostic)
	}
}

func min(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
