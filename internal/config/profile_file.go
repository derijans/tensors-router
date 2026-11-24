package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"tensors-router/internal/atomicfile"
)

const securitySectionHeader = "security:"

var profileLinePattern = regexp.MustCompile(`^(\s+profile:\s*)("[^"]*"|'[^']*'|[^\s#]*)(.*)$`)

func (cfg Config) SecureProfileBlocker(stored StoredCredentials) error {
	secured := cfg
	secured.Security.Profile = SecurityProfileSecure
	_, err := secured.CheckStoredCredentials(stored)
	return err
}

func WriteSecurityProfile(path string, profile string) error {
	if profile != SecurityProfileSecure && profile != SecurityProfileTrustedLAN {
		return fmt.Errorf("security.profile must be secure or trusted_lan")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return atomicfile.Write(path, []byte(withSecurityProfile(string(content), profile)), info.Mode().Perm())
}

func withSecurityProfile(content string, profile string) string {
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	profileLine := fmt.Sprintf("  profile: %q", profile)
	sectionIndex := securitySectionIndex(lines)
	if sectionIndex < 0 {
		return strings.Join(append([]string{securitySectionHeader, profileLine}, lines...), newline)
	}
	for index := sectionIndex + 1; index < len(lines) && !startsTopLevelSection(lines[index]); index++ {
		if match := profileLinePattern.FindStringSubmatch(lines[index]); match != nil {
			lines[index] = fmt.Sprintf("%s%q%s", match[1], profile, match[3])
			return strings.Join(lines, newline)
		}
	}
	lines = append(lines[:sectionIndex+1], append([]string{profileLine}, lines[sectionIndex+1:]...)...)
	return strings.Join(lines, newline)
}

func securitySectionIndex(lines []string) int {
	for index, line := range lines {
		if strings.TrimRight(stripTrailingComment(line), " ") == securitySectionHeader {
			return index
		}
	}
	return -1
}

func startsTopLevelSection(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(line, " ")
}

func stripTrailingComment(line string) string {
	before, _, _ := strings.Cut(line, " #")
	return before
}
