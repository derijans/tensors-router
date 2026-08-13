package openai

import (
	"testing"

	"tensors-router/internal/catalog"
)

func TestModelsResponseIncludesUnambiguousServedNamesAndAdapters(t *testing.T) {
	response := ModelsResponseFromCatalog([]catalog.Model{
		{ID: "first", ServedNames: []string{"public-first", "adapter-one", "shared"}},
		{ID: "second", ServedNames: []string{"shared", "first"}},
	})
	identifiers := make([]string, 0, len(response.Data))
	for _, model := range response.Data {
		identifiers = append(identifiers, model.ID)
	}
	want := []string{"first", "public-first", "adapter-one", "second"}
	if len(identifiers) != len(want) {
		t.Fatalf("unexpected merged model catalog: %#v", identifiers)
	}
	for index := range want {
		if identifiers[index] != want[index] {
			t.Fatalf("unexpected merged model catalog: %#v", identifiers)
		}
	}
}

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
