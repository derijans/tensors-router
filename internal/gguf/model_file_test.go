package gguf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitShardPathsListEveryShardInTheSameDirectory(t *testing.T) {
	directory := filepath.Join("models", "Big")
	shards := splitShardPaths(filepath.Join(directory, "Model-Q4-00001-of-00003.gguf"))
	want := []string{
		filepath.Join(directory, "Model-Q4-00001-of-00003.gguf"),
		filepath.Join(directory, "Model-Q4-00002-of-00003.gguf"),
		filepath.Join(directory, "Model-Q4-00003-of-00003.gguf"),
	}
	if len(shards) != len(want) {
		t.Fatalf("shards=%v", shards)
	}
	for index := range want {
		if shards[index] != want[index] {
			t.Fatalf("shard %d=%q, want %q keeping the exact case", index, shards[index], want[index])
		}
	}
}

func TestSplitShardPathsTreatOtherNamesAsOneFile(t *testing.T) {
	for _, name := range []string{"model.gguf", "model-00001-of-00000.gguf", "model-00001-of-00999.gguf", "model-00001-of-00002.GGUF"} {
		if shards := splitShardPaths(name); len(shards) != 1 || shards[0] != name {
			t.Fatalf("%s: shards=%v", name, shards)
		}
	}
}

func TestReadModelFileSumsShardSizes(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "m-00001-of-00002.gguf")
	if err := os.WriteFile(first, llamaHeader().bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "m-00002-of-00002.gguf"), make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	model, err := ReadModelFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if model.FileBytes != int64(len(llamaHeader().bytes()))+1000 || model.Metadata.Architecture != "llama" {
		t.Fatalf("model=%+v", model)
	}
}
