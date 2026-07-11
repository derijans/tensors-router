package backendendpoint

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func ParseLoopback(rawURL string) (*url.URL, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("backend URL scheme must be http or https")
	}
	if parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("backend URL host is invalid")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" {
		return parsed, nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("backend URL must use a loopback host")
	}
	return parsed, nil
}

func RejectConflictingArgs(args []string, flags ...string) error {
	for _, argument := range args {
		argument = strings.TrimSpace(argument)
		for _, flag := range flags {
			if argument == flag || strings.HasPrefix(argument, flag+"=") {
				return fmt.Errorf("extra_args cannot override %s", flag)
			}
		}
	}
	return nil
}
