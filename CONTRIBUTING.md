# Contributing

Thanks for helping. This page covers building, testing, and sending changes. [docs/Architecture.md](docs/Architecture.md) maps the code.

## Build

You need Go (the version in `go.mod`) and Node.js 24.

```bash
cd webui && npm ci && cd ..
make build
```

`make build` compiles the WebUI first, because the WebUI binary embeds it. On Windows, run `make` from Git Bash, or run the `go` and `npm` commands directly.

## Test

Run the same checks as CI before you open a pull request:

```bash
cd webui && npm run check && cd ..
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run ./...
go test ./...
go test -race ./internal/routerstore ./internal/recipes ./internal/benchmark ./internal/catalog ./internal/proxy ./internal/vllm ./internal/portalloc ./internal/backendendpoint ./internal/native ./internal/kobold ./internal/transportbody ./internal/cluster ./internal/auth
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
```

Run one test:

```bash
go test ./internal/proxy -run TestServiceRoutesImageRequest -v
```

Fuzz the streaming JSON processor after changing it:

```bash
go test ./internal/transportbody -run '^$' -fuzz FuzzProcessJSONMatchesEncodingJSON -fuzztime 5m
```

Commit any failing input that the fuzzer writes under `internal/transportbody/testdata/fuzz`, together with the fix.

## Tests that count

- Add a regression test that fails without your fix. Check that it fails before you apply the fix.
- Never change a test so that it passes on defective behavior.
- Put Go tests next to the code as `*_test.go`, and WebUI tests in `webui/src/test`.

## Style

- Format Go with `gofmt`. Keep files small and grouped by responsibility.
- Prefer names that explain the code over comments.
- In the WebUI, build markup with the `html` template from `safe-html.ts` and render it with `setHTML`. Do not use inline styles.
- Documentation must not use em dashes.

## Commits and pull requests

- Use a short, imperative subject, optionally scoped: `proxy: reject unavailable backend`.
- Explain the behavior change, how you validated it, and any config or deployment impact.
- Include screenshots for visible WebUI changes.
- Never commit secrets or local configuration (`config*.yaml`, `.kcpps/`, data directories).
