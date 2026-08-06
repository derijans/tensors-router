package loadcapture

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStoreCompletesReuseAndReconcilesInterruptedLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "captures.sqlite")
	store, err := NewStore(StoreConfig{NodeID: "node-a", DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		directoryInfo, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		databaseInfo, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if directoryInfo.Mode().Perm() != 0o700 || databaseInfo.Mode().Perm() != 0o600 {
			t.Fatalf("unexpected store permissions: directory=%o database=%o", directoryInfo.Mode().Perm(), databaseInfo.Mode().Perm())
		}
	}
	snapshot := Snapshot{SHA256: strings.Repeat("a", 64), JSON: []byte(`{"model_param":"sha256:abc"}`), Assets: []Asset{{Role: "model_param", Position: 0, SHA256: strings.Repeat("b", 64)}}}
	attempt, err := store.BeginPhysical(context.Background(), snapshot, "llama_sdcpp", "llama", "llm")
	if err != nil {
		t.Fatal(err)
	}
	capture := Capture{CapturedBytes: int64(len("C:/models/alpha.ggufrouter-secret")), Secrets: []string{"router-secret"}, Chunks: []Chunk{{Sequence: 1, Stream: StreamStdout, Offset: time.Millisecond, Payload: []byte("C:/models/")}, {Sequence: 2, Stream: StreamStdout, Offset: 2 * time.Millisecond, Payload: []byte("alpha.gguf")}, {Sequence: 3, Stream: StreamStderr, Offset: 3 * time.Millisecond, Payload: []byte("router-secret")}}}
	if err := store.CompletePhysical(context.Background(), attempt, nil, capture, map[string]string{"C:/models/alpha.gguf": "sha256:abc"}); err != nil {
		t.Fatal(err)
	}
	detail, err := store.Detail(context.Background(), attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Attempt.Status != StatusSucceeded || string(detail.Snapshot.JSON) != string(snapshot.JSON) || len(detail.Assets) != 1 {
		t.Fatalf("unexpected detail: %#v", detail)
	}
	output, err := store.Output(context.Background(), attempt.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	var persisted strings.Builder
	for _, chunk := range output.Chunks {
		persisted.Write(chunk.Payload)
	}
	if strings.Contains(persisted.String(), "C:/models") || strings.Contains(persisted.String(), "router-secret") || !strings.Contains(persisted.String(), "sha256:abc") || !strings.Contains(persisted.String(), "[REDACTED]") {
		t.Fatalf("output was not safely persisted: %#v", output)
	}
	reuse, err := store.RecordReuse(context.Background(), attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	reusedOutput, err := store.Output(context.Background(), reuse.ID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(reusedOutput.Chunks) != 1 || reusedOutput.NextSequence != reusedOutput.Chunks[0].Sequence {
		t.Fatalf("unexpected reused output page: %#v", reusedOutput)
	}
	if reuse.Kind != KindReuse || reuse.Status != StatusReused || reuse.PhysicalAttemptID != attempt.ID {
		t.Fatalf("unexpected reuse: %#v", reuse)
	}
	loading, err := store.BeginPhysical(context.Background(), snapshot, "kobold", "koboldcpp", "llm")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(StoreConfig{NodeID: "node-a", DatabasePath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	interrupted, err := reopened.Detail(context.Background(), loading.ID)
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.Attempt.Status != StatusInterrupted {
		t.Fatalf("expected interruption reconciliation, got %#v", interrupted.Attempt)
	}
	if err := reopened.CompletePhysical(context.Background(), attempt, errors.New("late completion"), Capture{}, nil); err != nil {
		t.Fatal(err)
	}
}
