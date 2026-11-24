package proxy

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"

	"tensors-router/internal/certgen"
)

var errNodeRequestSubjectChanged = errors.New("node returned a certificate request for other names or hosts than requested")

func verifyNodeCertificateRequest(requestPEM string, name string, hosts []string) error {
	request, err := certgen.ParseCertificateRequest([]byte(requestPEM))
	if err != nil {
		return fmt.Errorf("node returned an unreadable certificate request: %w", err)
	}
	if request.Subject.CommonName != strings.TrimSpace(name) {
		return errNodeRequestSubjectChanged
	}
	if len(hosts) == 0 {
		return nil
	}
	dnsNames, ipAddresses := certgen.SplitHosts(hosts)
	if !sameFoldedSet(request.DNSNames, dnsNames) || !sameFoldedSet(ipStrings(request.IPAddresses), ipStrings(ipAddresses)) {
		return errNodeRequestSubjectChanged
	}
	return nil
}

func sameFoldedSet(left []string, right []string) bool {
	return slices.Equal(sortedFolded(left), sortedFolded(right))
}

func sortedFolded(values []string) []string {
	folded := make([]string, 0, len(values))
	for _, value := range values {
		folded = append(folded, strings.ToLower(value))
	}
	slices.Sort(folded)
	return slices.Compact(folded)
}

func ipStrings(addresses []net.IP) []string {
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, address.String())
	}
	return values
}
