package update

import _ "embed"

//go:embed trusted-root.json
var embeddedTrustedRoot string

func TrustedRoot() []byte {
	return []byte(embeddedTrustedRoot)
}
