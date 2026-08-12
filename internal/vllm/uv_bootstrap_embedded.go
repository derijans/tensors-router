//go:build vllm_embedded_uv

package vllm

import _ "embed"

//go:embed assets/uv
var embeddedUV []byte
