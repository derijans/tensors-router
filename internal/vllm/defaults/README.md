# Embedded default vLLM manifests

Files named `<os>-<arch>.json` in this directory are compiled into the router and the
vLLM companion, and are used as the last-resort manifest tier when no TUF-authorized
`runtimes/vllm/<os>-<arch>.json` target has been published and no operator-pinned
manifest is configured.

Supported names:

- `linux-amd64.json`
- `linux-arm64.json`
- `darwin-arm64.json`

Each file must satisfy `ValidateManifest`. Every artifact carries an exact size and
SHA-256, so an embedded default still pins the bytes it installs. It is nevertheless a
weaker guarantee than TUF: it is not independently signed, it does not expire, and it
cannot be revoked without shipping a new release. The router reports the
`embedded-default` trust tier and logs a warning whenever this tier is used.

A directory containing no manifests is valid; the embedded tier is then simply
unavailable and the failure names the missing platform.
