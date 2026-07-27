package downloader

import "testing"

func TestSmartPlanKeepsRequiredDependencies(t *testing.T) {
	details := RepositoryDetails{Repository: "owner/model", Revision: "main", Commit: "0123456789abcdef0123456789abcdef01234567", Files: []File{
		{Path: "model-00001-of-00002.gguf", Size: 10},
		{Path: "model-00002-of-00002.gguf", Size: 10},
		{Path: "mmproj-model.gguf", Size: 5},
	}}
	plan, err := BuildPlan(details, []string{"model-00001-of-00002.gguf"}, "smart", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 3 {
		t.Fatalf("expected dependencies, got %#v", plan.Files)
	}
	for _, file := range plan.Files {
		if file.Path != "model-00001-of-00002.gguf" && !file.Required {
			t.Fatalf("expected %q to be required", file.Path)
		}
	}
	plan, err = BuildPlan(details, []string{"model-00001-of-00002.gguf"}, "explicit", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 {
		t.Fatalf("explicit plan should not add dependencies %#v", plan.Files)
	}
}
