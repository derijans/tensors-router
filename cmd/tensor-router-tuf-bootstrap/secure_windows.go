//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func secureDirectory(path string) error {
	user := strings.TrimSpace(os.Getenv("USERNAME"))
	domain := strings.TrimSpace(os.Getenv("USERDOMAIN"))
	if user == "" {
		return fmt.Errorf("USERNAME is required to restrict bootstrap key custody")
	}
	identity := user
	if domain != "" {
		identity = domain + `\\` + user
	}
	command := exec.Command("icacls.exe", path, "/inheritance:r", "/grant:r", identity+":(OI)(CI)F")
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("restrict bootstrap key custody ACL: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
