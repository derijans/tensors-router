package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxSecurityInputBytes = 1 << 20

type hostList []string

func (hosts *hostList) String() string {
	return strings.Join(*hosts, ",")
}

func (hosts *hostList) Set(value string) error {
	for _, host := range strings.Split(value, ",") {
		if host = strings.TrimSpace(host); host != "" {
			*hosts = append(*hosts, host)
		}
	}
	return nil
}

func readSecurityInput(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	var reader io.Reader = os.Stdin
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer file.Close()
		reader = file
	}
	content, err := io.ReadAll(io.LimitReader(reader, maxSecurityInputBytes+1))
	if err != nil {
		return "", err
	}
	if len(content) > maxSecurityInputBytes {
		return "", fmt.Errorf("%s is larger than %d bytes", path, maxSecurityInputBytes)
	}
	return string(content), nil
}

func writeNewPrivateFile(path string, content string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s already exists; refusing to overwrite it", path)
	}
	if err != nil {
		return err
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func requireFlag(name string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("--%s is required", name)
	}
	return nil
}
