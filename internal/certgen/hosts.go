package certgen

import (
	"net"
	"strings"
)

func SubjectFromHosts(commonName string, hosts []string) Subject {
	dnsNames, ipAddresses := SplitHosts(hosts)
	return Subject{CommonName: commonName, DNSNames: dnsNames, IPAddresses: ipAddresses}
}

func SplitHosts(hosts []string) ([]string, []net.IP) {
	dnsNames := []string{}
	ipAddresses := []net.IP{}
	seenDNS := map[string]struct{}{}
	seenIP := map[string]struct{}{}
	for _, value := range hosts {
		value = hostWithoutPort(value)
		if value == "" || IsWildcardHost(value) {
			continue
		}
		if ip := net.ParseIP(value); ip != nil {
			key := ip.String()
			if _, ok := seenIP[key]; !ok {
				seenIP[key] = struct{}{}
				ipAddresses = append(ipAddresses, ip)
			}
			continue
		}
		dnsNameKey := strings.ToLower(value)
		if _, ok := seenDNS[dnsNameKey]; ok {
			continue
		}
		seenDNS[dnsNameKey] = struct{}{}
		dnsNames = append(dnsNames, value)
	}
	return dnsNames, ipAddresses
}

func LoopbackHosts() []string {
	return []string{"localhost", "127.0.0.1", "::1"}
}

func LocalInterfaceHosts() []string {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	values := []string{}
	for _, address := range addresses {
		switch typedAddress := address.(type) {
		case *net.IPNet:
			if typedAddress.IP != nil {
				values = append(values, typedAddress.IP.String())
			}
		case *net.IPAddr:
			if typedAddress.IP != nil {
				values = append(values, typedAddress.IP.String())
			}
		}
	}
	return values
}

func BindHost(bind string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(bind))
	if err != nil {
		return UnbracketHost(bind)
	}
	return UnbracketHost(host)
}

func IsWildcardHost(host string) bool {
	host = UnbracketHost(host)
	return host == "" || host == "0.0.0.0" || host == "::"
}

func UnbracketHost(host string) string {
	host = strings.TrimSpace(host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	return host
}

func hostWithoutPort(value string) string {
	value = UnbracketHost(value)
	if host, _, err := net.SplitHostPort(value); err == nil {
		return UnbracketHost(host)
	}
	return value
}
