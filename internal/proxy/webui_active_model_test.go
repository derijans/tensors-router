package proxy

import "testing"

func TestRemoteActiveWebUIModelNeverReplacesTheLocalOne(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		localActive bool
		wantActive  string
	}{
		{name: "local active", localActive: true, wantActive: "qwen2.5-1.5b"},
		{name: "remote only", localActive: false, wantActive: "qwen25-05b"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			entry := WebUIEntry{CompatibleModels: []WebUICompatibleModel{
				{ModelID: "qwen2.5-1.5b", NodeID: "master", Filename: "qwen2.5-1.5b.kcpps", Active: testCase.localActive},
				{ModelID: "qwen25-05b", NodeID: "slave", Filename: "qwen25-05b.kcpps"},
			}}
			if testCase.localActive {
				entry.Active = true
				entry.ActiveModelID = "qwen2.5-1.5b"
			}
			remote := WebUIEntry{Active: true, CompatibleModels: []WebUICompatibleModel{
				{ModelID: "qwen25-05b", NodeID: "slave", Filename: "qwen25-05b.kcpps", Active: true},
			}}

			markRemoteActiveWebUIEntry(&entry, remote)

			if !entry.Active || !entry.CompatibleModels[1].Active || entry.ActiveModelID != testCase.wantActive {
				t.Fatalf("active=%t remote model active=%t active model=%q, want %q", entry.Active, entry.CompatibleModels[1].Active, entry.ActiveModelID, testCase.wantActive)
			}
		})
	}
}
