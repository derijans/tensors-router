package cluster

import (
	"testing"

	"tensors-router/internal/catalog"
)

func TestLocalModelsExposeOnlyUnambiguousServedNames(t *testing.T) {
	models := []catalog.Model{
		{ID: "first", BackendMode: BackendModeVLLM, HasLLM: true, ServedNames: []string{"shared", "first-alias"}},
		{ID: "second", BackendMode: BackendModeVLLM, HasLLM: true, ServedNames: []string{"shared", "first"}},
	}

	records := LocalModelsWithBackendMode(models, "local", "", SourceLocal, BackendModeKobold)
	ids := make(map[string]int)
	for _, record := range records {
		ids[record.PublicID]++
	}
	if ids["first"] != 1 || ids["second"] != 1 || ids["first-alias"] != 1 {
		t.Fatalf("expected primary and unique served names: %#v", ids)
	}
	if ids["shared"] != 0 {
		t.Fatalf("ambiguous served name was exposed: %#v", ids)
	}
}
