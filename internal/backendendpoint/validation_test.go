package backendendpoint

import "testing"

func TestParseLoopbackAcceptsOnlyLoopbackHosts(t *testing.T) {
	for _, rawURL := range []string{
		"http://127.0.0.1:5001",
		"http://[::1]:5001",
		"https://localhost:5001",
	} {
		if _, err := ParseLoopback(rawURL); err != nil {
			t.Fatalf("expected %s to be accepted: %v", rawURL, err)
		}
	}
	for _, rawURL := range []string{
		"http://0.0.0.0:5001",
		"http://192.168.1.20:5001",
		"http://backend.internal:5001",
		"ftp://127.0.0.1:5001",
	} {
		if _, err := ParseLoopback(rawURL); err == nil {
			t.Fatalf("expected %s to be rejected", rawURL)
		}
	}
}

func TestRejectConflictingArgsRecognizesSeparateAndEqualsForms(t *testing.T) {
	for _, argument := range []string{"--host", "--host=0.0.0.0", "--listen-ip", "--listen-ip=::"} {
		if err := RejectConflictingArgs([]string{argument}, "--host", "--listen-ip"); err == nil {
			t.Fatalf("expected %q to be rejected", argument)
		}
	}
	if err := RejectConflictingArgs([]string{"--parallel", "2"}, "--host", "--listen-ip"); err != nil {
		t.Fatal(err)
	}
}
