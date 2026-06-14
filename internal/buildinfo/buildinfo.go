package buildinfo

import (
	"runtime/debug"
	"strings"
)

var Version string
var Commit string
var Date string

type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	Date    string `json:"date,omitempty"`
}

func Current() Info {
	revision, revisionTime := vcsInfo()
	commit := firstNonEmpty(Commit, revision)
	version := firstNonEmpty(Version, shortCommit(commit), "dev")
	return Info{
		Version: version,
		Commit:  commit,
		Date:    firstNonEmpty(Date, revisionTime),
	}
}

func (info Info) String() string {
	values := []string{"version=" + firstNonEmpty(info.Version, "dev")}
	if strings.TrimSpace(info.Commit) != "" && strings.TrimSpace(info.Commit) != strings.TrimSpace(info.Version) {
		values = append(values, "commit="+strings.TrimSpace(info.Commit))
	}
	if strings.TrimSpace(info.Date) != "" {
		values = append(values, "built="+strings.TrimSpace(info.Date))
	}
	return strings.Join(values, " ")
}

func vcsInfo() (string, string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", ""
	}
	revision := ""
	revisionTime := ""
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.time":
			revisionTime = strings.TrimSpace(setting.Value)
		}
	}
	return revision, revisionTime
}

func shortCommit(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
