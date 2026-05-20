package openai

import "testing"

func TestModelFromJSON(t *testing.T) {
	model, ok, err := ModelFromJSON([]byte(`{"model":"a.kcpps","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !ok || model != "a.kcpps" {
		t.Fatalf("unexpected model %q %v", model, ok)
	}
}

func TestModelFromJSONMissingModel(t *testing.T) {
	_, ok, err := ModelFromJSON([]byte(`{"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("model should be missing")
	}
}

func TestModelFromJSONRejectsNonStringModel(t *testing.T) {
	_, _, err := ModelFromJSON([]byte(`{"model":123}`))
	if err == nil {
		t.Fatalf("expected error")
	}
}
