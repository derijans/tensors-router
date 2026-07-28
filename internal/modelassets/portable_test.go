package modelassets

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const testHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestExportAndResolvePortableConfig(t *testing.T) {
	exported, err := Export([]byte(`{"model_param":"C:/models/main.gguf","sdlora":["C:/models/a.safetensors","C:/models/b.safetensors"]}`), func(path string) (string, error) { return testHash, nil }, func(string) (Origin, bool) {
		return Origin{Repository: "owner/repo", Commit: "0123456789abcdef0123456789abcdef01234567", Path: "main.gguf"}, true
	})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(exported, &config); err != nil {
		t.Fatal(err)
	}
	if _, found := config["model_param"]; found {
		t.Fatal("path was retained")
	}
	if config["model_param_filename"] != "main.gguf" {
		t.Fatalf("unexpected filename %#v", config)
	}
	resolved, err := Resolve(exported, func(hash string, filename string) (string, bool) { return "D:/shared/" + filename, hash == testHash })
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Fields) != 3 {
		t.Fatalf("unexpected fields %#v", resolved.Fields)
	}
	config = nil
	if err := json.Unmarshal(resolved.Content, &config); err != nil {
		t.Fatal(err)
	}
	if config["model_param"] != "D:/shared/main.gguf" {
		t.Fatalf("unexpected resolved config %#v", config)
	}
	if _, found := config["model_param_hash"]; found {
		t.Fatal("hash was retained")
	}
}

func TestExportRejectsMissingAndUnsafeValues(t *testing.T) {
	_, err := Export([]byte(`{"model_param":"C:/models/main.gguf"}`), func(string) (string, error) { return "", errors.New("missing") }, nil)
	if err == nil {
		t.Fatal("missing model was accepted")
	}
	_, err = Resolve([]byte(`{"model_param_hash":"ABC","model_param_filename":"x.gguf"}`), func(string, string) (string, bool) { return "", false })
	if err == nil {
		t.Fatal("invalid hash was accepted")
	}
	_, err = Resolve([]byte(`{"model_param":"x","model_param_hash":"`+testHash+`","model_param_filename":"x.gguf"}`), func(string, string) (string, bool) { return "", false })
	if err == nil {
		t.Fatal("mixed forms were accepted")
	}
}

func TestPortableSchemaRejectsTraversalDevicesUnknownFieldsAndMismatchedArrays(t *testing.T) {
	cases := []string{
		`{"model_param_hash":"` + testHash + `","model_param_filename":"../model.gguf"}`,
		`{"model_param_hash":"` + testHash + `","model_param_filename":"CON"}`,
		`{"unknown_hash":"` + testHash + `","unknown_filename":"model.gguf"}`,
		`{"sdlora_hash":["` + testHash + `","` + testHash + `"],"sdlora_filename":["one.gguf"]}`,
		`{"model_param_hash":"` + testHash + `","model_param_filename":"model.gguf","model_param_hf":"https://example.com/model"}`,
	}
	for _, content := range cases {
		if _, err := Resolve([]byte(content), func(string, string) (string, bool) { return "", false }); err == nil {
			t.Fatalf("unsafe portable config was accepted: %s", content)
		}
	}
}

func TestResolveRetainsUnavailableField(t *testing.T) {
	resolved, err := Resolve([]byte(`{"model_param_hash":"`+testHash+`","model_param_filename":"main.gguf"}`), func(string, string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Fields) != 1 || resolved.Fields[0].Failure == "" {
		t.Fatalf("unexpected results %#v", resolved.Fields)
	}
	if string(resolved.Content) == "" {
		t.Fatal("missing portable content")
	}
}

func TestSubstituteRequiresExpectedHashAndPreservesArrayShape(t *testing.T) {
	original := strings.Repeat("a", 64)
	replacement := strings.Repeat("b", 64)
	position := 1
	content := []byte(`{"sdlora_hash":["` + original + `","` + original + `"],"sdlora_filename":["one.gguf","two.gguf"],"sdlora_hf":[null,null]}`)
	origin := Origin{Repository: "owner/repository", Commit: strings.Repeat("c", 40), Path: "models/replacement.gguf"}
	updated, err := Substitute(content, "sdlora", &position, original, replacement, "replacement.gguf", origin)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(updated, &config); err != nil {
		t.Fatal(err)
	}
	hashes := config["sdlora_hash"].([]any)
	filenames := config["sdlora_filename"].([]any)
	hfs := config["sdlora_hf"].([]any)
	if hashes[0] != original || hashes[1] != replacement || filenames[1] != "replacement.gguf" || hfs[1] != origin.URI() {
		t.Fatalf("unexpected substitution: %s", updated)
	}
	if _, err := Substitute(content, "sdlora", &position, strings.Repeat("d", 64), replacement, "replacement.gguf", origin); err == nil {
		t.Fatal("expected stale hash substitution to fail")
	}
}
