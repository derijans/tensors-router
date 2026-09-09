package routerstore

import "strings"

const sharedPragmas = "_pragma=busy_timeout(5000)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=synchronous(NORMAL)" +
	"&_pragma=foreign_keys(1)"

func writerDSN(path string) string {
	return path + "?" + sharedPragmas + "&_txlock=immediate"
}

func readerDSN(path string) string {
	return path + "?" + sharedPragmas
}

func pathAcceptedByDriver(path string) bool {
	return !strings.Contains(path, "?")
}
