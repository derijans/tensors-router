package downloader

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

const defaultHubEndpoint = "https://huggingface.co"

func resolveHubEndpoint(configured string) (string, error) {
	value := strings.TrimSpace(configured)
	if value == "" {
		value = strings.TrimSpace(os.Getenv("HF_ENDPOINT"))
	}
	if value == "" {
		return defaultHubEndpoint, nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("huggingface.endpoint must be an absolute URL without credentials, query, or fragment")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopbackHost(parsed.Hostname())) {
		return "", fmt.Errorf("huggingface.endpoint must use https (plain http is allowed only for loopback mirrors)")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func environmentHubToken() string {
	for _, name := range []string{"HF_TOKEN", "HUGGING_FACE_HUB_TOKEN"} {
		if token := strings.TrimSpace(os.Getenv(name)); token != "" {
			return token
		}
	}
	return ""
}
