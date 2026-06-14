package buildinfo

import "testing"

func TestInfoStringIncludesVersionCommitAndDate(t *testing.T) {
	info := Info{
		Version: "v1.2.3",
		Commit:  "abcdef123456",
		Date:    "2026-06-14T12:00:00Z",
	}
	if got := info.String(); got != "version=v1.2.3 commit=abcdef123456 built=2026-06-14T12:00:00Z" {
		t.Fatalf("unexpected build string %q", got)
	}
}

func TestCurrentUsesInjectedVersionOrCommit(t *testing.T) {
	oldVersion := Version
	oldCommit := Commit
	oldDate := Date
	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
		Date = oldDate
	})

	Version = ""
	Commit = "1234567890abcdef"
	Date = ""

	info := Current()
	if info.Version != "1234567890ab" {
		t.Fatalf("expected short commit version, got %#v", info)
	}
	if info.Commit != "1234567890abcdef" {
		t.Fatalf("expected full commit, got %#v", info)
	}
}
