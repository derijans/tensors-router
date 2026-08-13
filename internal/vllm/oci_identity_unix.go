//go:build !windows

package vllm

import (
	"os"
	"strconv"
)

func containerIdentityArguments(engine string) []string {
	if engine == "podman" {
		return []string{"--userns", "keep-id", "--group-add", "keep-groups"}
	}
	arguments := []string{"--user", strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())}
	groups, err := os.Getgroups()
	if err != nil {
		return arguments
	}
	for _, group := range groups {
		if group != os.Getgid() {
			arguments = append(arguments, "--group-add", strconv.Itoa(group))
		}
	}
	return arguments
}
