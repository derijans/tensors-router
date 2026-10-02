package downloader

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSupervisorRestartsCrashedCompanionAndForwardsArtifactEvents(t *testing.T) {
	binaryPath, configPath := buildCompanion(t)
	supervisor, err := StartSupervisedClient(context.Background(), binaryPath, configPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = supervisor.Close() })
	artifacts := make(chan ArtifactRecord, 4)
	supervisor.SetArtifactHandler(func(record ArtifactRecord) error {
		artifacts <- record
		return nil
	})
	supervisor.mu.Lock()
	crashed := supervisor.current
	supervisor.mu.Unlock()
	if err := crashed.command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-crashed.Done()
	if _, err := supervisor.Jobs(); ErrorCodeOf(err) != ErrorServiceUnavailable {
		t.Fatalf("calls during the restart window must report the outage, got %v", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := supervisor.Jobs(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("crashed downloader companion was never restarted")
		}
		time.Sleep(100 * time.Millisecond)
	}
	storageRoot := filepath.Join(filepath.Dir(configPath), "models")
	if err := os.WriteFile(filepath.Join(storageRoot, "manual.gguf"), []byte("weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := supervisor.Rescan(); err != nil {
		t.Fatal(err)
	}
	select {
	case record := <-artifacts:
		if filepath.Base(record.Path) != "manual.gguf" || record.SHA256 == "" {
			t.Fatalf("unexpected artifact event %#v", record)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restarted companion did not push the artifact event to the router handler")
	}
}
