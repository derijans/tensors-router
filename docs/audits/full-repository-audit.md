# Full Repository Audit, Remediation, and Multi-Node Evidence Report

Audit date: 2026-08-07  
Audited revision: `a71ea9479b0d9a1c2d3729c39aa769a0c9495182` (`bug fixes`, 2026-08-06 21:53:44 +0300)  
Scope: repository root, three runtime executables, two TUF operator commands, all root Go packages, browser application, deployment definitions, operational scripts, persistence, and published documentation  
Report lifecycle: original audit evidence, merged remediation, operational activation, and release revalidation

## Executive summary

This document preserves the baseline evidence from the audit of revision `a71ea9479b0d9a1c2d3729c39aa769a0c9495182` and records the remediation merged in `e0bf28fd7b0698f0cb0429c5b9c9f660988a0687`. The insecure behaviors behind TRA-2026-001 through TRA-2026-007 are removed in code: executable downloads now require a direct SHA-256 pin or a valid TUF authorization; shipped deployments authenticate published services; cluster identities are immutable while live; cluster control calls are bounded and concurrent; the downloader is a self-contained Go companion; systemd allows the configured drain; and executable promotion is transactional.

TRA-2026-001 and TRA-2026-007 are operationally active for the default repository. GitHub Pages serves the public TUF repository, the `tuf-publication` environment permits deployments only from `main`, and its encrypted secrets contain only the delegated upstream-targets, snapshot, and timestamp keys. Publication run `31183299695` independently downloaded and hashed the allowlisted upstream assets, signed versioned metadata, published timestamp last, and passed its clean-cache verification. A second clean go-tuf v2 client fetched the live Pages endpoint with the embedded root and verified all eight signed target manifests.

The Ollama compatibility review is also complete. Official route methods and `Allow` responses are enforced, all Ollama failures use `{"error":"message"}`, streamed NDJSON records are bounded and rewritten independently, and `/api/tags` and `/api/ps` synthesize deduplicated public model identities without leaking backend-local IDs.

The final Go suite, `go vet`, WebUI audit/lint/tests/type-check/build, repeated high-risk suites, cross-builds, archive smoke checks, YAML/JSON parsing, and the pinned reachable-vulnerability scan pass locally. GitHub run `31183274035` additionally passes the targeted race suite, five-platform builds, container builds, deployment rendering, image-boundary checks, and reachable-vulnerability scan on the exact merged revision. GitHub code scanning reports no new alerts in the remediated pull request.

## Prioritized action table

| Priority | Finding | Severity | Remediation status | Operational status |
| --- | --- | --- | --- | --- |
| 1 | TRA-2026-001: updater installs payloads without an authenticated digest | High | Resolved and regression-tested | Default signed TUF service active and clean-client verified |
| 2 | TRA-2026-002: shipped LAN deployment exposes administrative mutation without authentication | High | Resolved and regression-tested | Secure defaults and bootstrap helper ready |
| 3 | TRA-2026-004: cluster JSON timeout is disabled and health checks are serialized | High | Resolved and regression-tested | Ready |
| 4 | TRA-2026-003: duplicate cluster identities silently replace nodes | High | Resolved and regression-tested | Ready |
| 5 | TRA-2026-005: container downloader lacks its required `hf` executable | High | Resolved and regression-tested | Self-contained node and WebUI container images build and pass boundary checks in CI |
| 6 | TRA-2026-006: systemd stop timeout truncates the configured drain | Medium | Resolved and generated-unit-tested | Ready |
| 7 | TRA-2026-007: updater removes the old executable before replacement succeeds | Medium | Resolved; Windows and Unix regressions pass | Default signed TUF service active |

## Audit execution plan artifact

| Phase | Intended evidence | Status |
| --- | --- | --- |
| Inventory | Executables, packages, WebUI modules, manifests, scripts, stores, docs, external integrations | Complete |
| Static audit | Correctness, security, lifecycle, persistence, deployment, and documentation review | Complete |
| Baseline verification | Go test/vet/coverage, WebUI check/audit, pinned `govulncheck` | Complete |
| Focused verification | Repetition, boundary, streaming, cancellation, concurrency, malformed input, asset transfer | Complete through tests and deterministic lab, with gaps listed below |
| Multi-node lab | Loopback master and two slaves with deterministic backends and controlled failure | Complete for registration, balancing, streaming, cancellation, authentication, and failure observation |
| Real backends | KoboldCpp text, KoboldCpp voice, SD 1.x image, split native lanes | Text and voice complete; SD and full split-native inference blocked |
| Performance | Repeated latency and process-resource samples with caveats | Partial but repeatable; no unsupported throughput claim |
| Report | Confirmed, rejected, and inconclusive candidates with fix designs | Complete |

## Remediation implementation plan artifact

| Phase | Outcome | Status |
| --- | --- | --- |
| Secure updater | Embedded TUF root, staged metadata, exact signed manifests, direct SHA pins, transactional promotion | Code complete |
| TUF publication | Bootstrap CLI, delegated online signer, scheduled publisher, independent hashing, clean-cache verification | Complete and operationally verified |
| Secure deployment | File-backed Compose secrets, direct Portainer credentials, role separation, secure profiles, trusted-LAN overlay | Complete |
| Cluster correctness | Normalized immutable identities, typed conflicts, terminal slave shutdown, bounded concurrent synchronization | Complete |
| Self-contained downloader | Framed stdio worker, native HTTP transfer/resume/hash/promotion, lifecycle handshake and cleanup | Complete |
| Ollama compatibility | Dedicated methods/errors/stream adapter and public cluster model discovery | Complete |
| Verification and audit | Full local suites, CI race/container checks, cross-builds, archive/config checks, GHAS, live TUF verification | Complete with documented upstream and real-backend limitations |

## Environment and repository state

| Item | Exact value |
| --- | --- |
| Host OS | Windows 11 build `10.0.26100.0`, `windows/amd64` |
| PowerShell | `5.1.26100.7462` |
| CPU | AMD Ryzen 9 9950X3D, 24 logical processors exposed |
| Physical memory | 15,708,110,848 bytes |
| Final free space on C: | 3,858,141,184 bytes |
| Go | `go1.26.5 windows/amd64`; root module now declares Go 1.25 |
| CGO/toolchain | `CGO_ENABLED=0`; `CC=gcc`, but neither `gcc` nor `clang` is installed |
| Node/npm | Node `v24.13.1`; npm `11.8.0` |
| Git | `2.52.0.windows.1` |
| Containers | Docker absent; Podman absent |
| Root direct Go dependency | `modernc.org/sqlite v1.29.10` |
| WebUI toolchain | Vite `8.0.16`, Vitest `4.1.8`, TypeScript `6.0.3`, ESLint `10.4.1`, typescript-eslint `8.61.0` |

At baseline, `.git/config` consisted of 481 NUL bytes and normal Git commands failed with `fatal: bad config line 1 in file .git/config`. The original audit therefore used `-buildvcs=false` and reconstructed revision provenance read-only from Git metadata. During remediation validation, `.git/config` was valid (334 bytes, no NUL bytes), `git rev-parse HEAD` resolved the same audited revision, and normal status/diff commands worked. The audit does not attribute the external repair.

The managed command sandbox could not launch because `codex-windows-sandbox-setup.exe` is absent. Verification commands were run with explicitly approved host execution. This is an audit-environment limitation, not a repository defect.

## Baseline verification

| Command | Result | Evidence |
| --- | --- | --- |
| `go test -buildvcs=false ./...` | Pass | All root-module packages; 41.7 seconds |
| `go vet -buildvcs=false ./...` | Pass | No diagnostics; 6.0 seconds |
| `go test -buildvcs=false -coverprofile=.tmp/audit-coverage.out ./...` | Pass | Total statement coverage 59.9% |
| `npm run check` in `webui` | Pass | audit, ESLint, 16 Vitest files/46 tests, TypeScript, and Vite build |
| `npm audit --audit-level=moderate` | Pass | 0 vulnerabilities |
| `go run -buildvcs=false golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...` | Pass | No reachable vulnerabilities |
| Pinned `govulncheck` verbose result | Informational | GO-2026-5024 in indirect `golang.org/x/sys/windows` v0.19.0, fixed in v0.44.0; no vulnerable symbol is called |
| `go test -buildvcs=false -count=10 ./internal/cluster ./internal/proxy ./internal/downloader ./internal/update ./internal/webui ./internal/transportbody` | Pass | Ten consecutive focused runs; 37.5 seconds |
| Race suite | Not run | Required C compiler is absent and CGO is disabled |

The exact race command to run in CI/Linux is:

```sh
go test -race ./internal/recipes ./internal/benchmark ./internal/catalog ./internal/proxy
```

Package coverage was strongest in `auth` (88.2%), `native` (80.2%), `config` (80.0%), `benchmark` (79.0%), `analytics` (74.2%), `catalog` (73.7%), and `cook` (72.4%). It was materially lower in the executable entry points (0–27.4%), `kobold` (35.6%), `proxy` (54.7%), `downloader` (55.5%), `modelassets` (57.8%), and `transportbody` (58.4%). `atomicfile`, `backendmode`, and `processcontrol` recorded 0% in the aggregate run; `siteapi` and the WebUI command have no direct tests. The raw profile is `.tmp/audit-coverage.out`.

## Repository coverage matrix

### Executables and Go packages

`Inspected + verified` means source review plus successful build/test/vet coverage. `Inspected + build/vet` identifies packages without a direct test suite.

| Area | Status | Audit emphasis |
| --- | --- | --- |
| `cmd/tensors-router` | Inspected + verified | configuration, startup ordering, signal handling, shutdown, companion integration |
| `cmd/tensor-router-webui` | Inspected + build/vet | TLS/session server and managed router lifecycle |
| `cmd/tensor-router-downloader` | Inspected + verified | protocol framing, process lifecycle, downloader command wiring |
| `internal/analytics` | Inspected + verified | SQLite lifecycle, response observation, shutdown flush |
| `internal/atomicfile` | Inspected + build/vet | replacement semantics and permissions |
| `internal/auth` | Inspected + verified | CIDR, route classes, inference/admin/cluster credentials |
| `internal/backenddiagnostic` | Inspected + verified | error capture and redaction |
| `internal/backendendpoint` | Inspected + verified | endpoint normalization and trust boundary |
| `internal/backendmode` | Inspected + build/vet | mode normalization |
| `internal/benchmark` | Inspected + verified | persistence, concurrency, measurement semantics |
| `internal/buildinfo` | Inspected + verified | build stamps and corrupt-Git effect |
| `internal/catalog` | Inspected + verified | discovery, mutation, identity, concurrent state |
| `internal/cluster` | Inspected + verified + lab | registry, identity, health, sync, auth, selection |
| `internal/companion` | Inspected + verified | framed child protocol and termination |
| `internal/config` | Inspected + verified | defaults, security profiles, update/downloader validation |
| `internal/cook` | Inspected + verified | generated options, file targets, backend compatibility |
| `internal/downloader` | Inspected + verified | Hub API, paths, hashing, resume, SQLite, child CLI |
| `internal/hardware` | Inspected + verified | inventory and platform probes |
| `internal/inventory` | Inspected + verified | file-root enforcement and redaction |
| `internal/kobold` | Inspected + verified + real backend | startup, reload, readiness, cancellation, process tree |
| `internal/loadcapture` | Inspected + verified | WAL, retention, serialization, shutdown |
| `internal/mcp` | Inspected + verified | generated MCP configuration and paths |
| `internal/modelassets` | Inspected + verified | hash cache, peer transfer, partial promotion, SQLite |
| `internal/native` | Inspected + verified | llama/sd/whisper lane lifecycle and readiness |
| `internal/openai` | Inspected + verified | JSON/error contract and response shape |
| `internal/processcontrol` | Inspected + build/vet | Windows process-tree termination |
| `internal/proxy` | Inspected + verified + lab | routing, retry, streaming, drain, rewrite, shared state |
| `internal/recipes` | Inspected + verified | persistence, routing plans, concurrent state |
| `internal/siteapi` | Inspected + build/vet | public site DTO boundary |
| `internal/transportbody` | Inspected + verified | replay/stream limits, cancellation, cleanup |
| `internal/unloadpolicy` | Inspected + verified | lane target validation |
| `internal/update` | Inspected + verified | source trust, extraction, hashing, replacement |
| `internal/webui` | Inspected + verified | TLS, sessions, CSRF, reverse proxy, process lifecycle |

### Browser application

All modules under `webui/src` were inspected and the complete WebUI check passed.

| Responsibility | Modules | Status |
| --- | --- | --- |
| Bootstrap, state, API, common data | `bootstrap.ts`, `main.ts`, `state.ts`, `api.ts`, `operations.ts`, `constants.ts`, `data.ts`, `utils.ts`, `json.ts`, `conversions.ts` | Inspected + verified |
| DOM and rendering | `dom.ts`, `elements.ts`, `render-dashboard.ts`, `dialogs.ts`, `terminal-output.ts`, `styles.css`, `dirty-state.ts` | Inspected + verified |
| Cook and constructor | `constructor.ts`, `constructor-data.ts`, `constructor-field-data.ts`, `constructor-field-editor.ts`, `constructor-options.ts`, `cook-actions.ts`, `cook-result.ts`, `simple-cook.ts`, `simple-cook-data.ts` | Inspected + verified |
| Downloads and model files | `downloads.ts`, `download-capability-data.ts`, `download-finder-data.ts`, `download-plan-data.ts`, `hf-filter-catalog.ts`, `model-actions.ts`, `model-files.ts`, `model-filter-data.ts` | Inspected + verified |
| Analytics, captures, benchmarks | `analytics.ts`, `analytics-data.ts`, `load-captures.ts`, `benchmarks.ts`, `benchmark-data.ts` | Inspected + verified |
| Backend WebUIs | `webuis.ts`, `webui-data.ts` | Inspected + verified |
| Types and tests | `types/` and all 16 files in `test/` | Inspected + verified; 46 tests passed |

Dynamic content is normally passed through escaping helpers before HTML insertion. No inline style was added by this audit, and no confirmed DOM injection path was found.

### Deployment, scripts, persistence, and documentation

| Area | Files or stores | Status |
| --- | --- | --- |
| Container build | `Containerfile` | Inspected; build/run blocked by absent Docker |
| Compose | `deploy/compose.base.yaml`, `compose.amd.yaml`, `compose.nvidia.yaml` | Inspected; schema/runtime validation blocked by absent Docker |
| Portainer | `templates.json`; node and WebUI CPU/AMD/NVIDIA manifests | All seven files inspected |
| Deployment configuration | `downloader.yaml`, `node.yaml`, `router-managed.yaml`, `webui.yaml` | All inspected |
| Operational scripts | all seven files in `scripts/` | All inspected; Windows Kobold smoke script executed |
| SQLite | analytics, benchmark, downloader, load capture, model assets, recipes | Schema, pragmas, open/close, concurrency, and migrations inspected; tests passed |
| File/JSON persistence | catalog, hash cache, MCP, generated configurations, update metadata | Inspected; tests passed where present |
| Wiki | all 20 Markdown pages under `docs/wiki` | Inspected for API, configuration, security, topology, backend, and deployment claims |
| Documentation images | all nine `docs/images/webui-live/*.png` assets | Presence and references inspected; visual content not re-baselined |
| External backends | KoboldCpp, llama-server, sd-server, whisper-server | Kobold text and TTS run; native source paths inspected; full split run incomplete |
| External services | Hugging Face API/CLI and GitHub release API | Input/trust/source handling inspected; no live destructive download initiated |

## Multi-node lab

### Topology

```text
client -> master 127.0.0.1:19100
             |-> slave-a 127.0.0.1:19101 -> deterministic backend :19201
             `-> slave-b 127.0.0.1:19102 -> deterministic backend :19202
```

All nodes used the `secure` profile, separate inference/admin/cluster test credentials, loopback-only binds, one-second sync and health intervals, identical 25-byte model fixtures, and distinct data/store directories under `.tmp/audit/lab`. The backend supported deterministic delay, forced status, malformed input, buffered responses, and SSE. Builds and fixtures remained ignored.

### Scenario matrix

| Scenario | Result | Evidence |
| --- | --- | --- |
| Master plus two slave startup and registration | Pass | Master catalog contained both replicas |
| Missing inference credential | Pass | `/v1/models` returned 401 |
| Missing cluster credential | Pass | `/router/v1/node/models` returned 401 |
| Public/local model ID rewriting | Pass | Master accepted `shared`; both slave-local backends saw the local ID and responses exposed the public ID |
| Round-robin selection | Pass | Six requests: `19201,19202,19201,19202,19201,19202` |
| Buffered request | Pass | JSON completion returned from both nodes |
| Streaming request | Pass | HTTP 200 and terminal `data: [DONE]` |
| Client cancellation | Pass | 2,000 ms delayed request canceled in 183.93 ms with a 150 ms client deadline |
| Slave process-tree termination | Pass | Killing slave A removed its router and child backend; slave B and master remained |
| Stable failover after slave A loss | Inconclusive/failed observation | One run returned no available replicas; a second served two requests on B then returned two 502 responses although B remained alive |
| Recovery-time measurement | Inconclusive | Failover instability prevented a trustworthy recovery percentile |
| Graceful shutdown/orphan absence | Pass in clean run | All three routers and deterministic children exited; no matching listeners/processes remained |
| Duplicate node ID | Confirmed defect | Second snapshot silently replaced the first; see TRA-2026-003 |
| Hanging cluster JSON response | Confirmed defect | Request remained blocked after 31 seconds; see TRA-2026-004 |
| Conflicting model hashes | Pass | Existing public-ID conflict indexing keeps distinct configurations addressable |
| Asset partial/resume/hash/concurrency | Pass in focused suites | Direct peer transfer, verified partial promotion, hash mismatch, and one-transfer concurrency tests passed |
| Real text directly and through router | Router path pass | Gemma completion returned `ready`; direct backend readiness was also observed by the smoke harness |
| Real voice through router | Pass | HTTP 200, 56,534 bytes, `RIFF` magic |
| Real SD 1.5 direct and through master | Blocked | Insufficient free disk for verified checkpoint and staging/runtime headroom |
| Split llama/sd/whisper inference | Blocked/partial | llama and sd binaries existed, no SD model or Whisper server; routing adapters covered by focused tests |

The repeated failover observation is not promoted to a separate confirmed finding because the precise state transition was not isolated. It is relevant supporting evidence for prioritizing bounded, independent health checks.

## Performance evidence

The deterministic backend performs negligible work, so the difference between direct and routed samples primarily measures HTTP, JSON rewrite, selection, and proxy overhead on this host. Samples were sequential and warmed before collection.

| Measurement | Samples | p50 | p95 | Notes |
| --- | ---: | ---: | ---: | --- |
| Direct deterministic backend | 40 | 0.824 ms | 1.222 ms | `127.0.0.1:19201` |
| Through master and slave | 40 | 3.456 ms | 4.738 ms | Public-ID rewrite and two HTTP hops |
| Approximate routing delta | 40 paired | 2.632 ms | 3.516 ms | Difference of reported percentiles, not per-pair percentile |
| First cold routed deterministic request | 1 | 29.45 ms | — | Includes lazy stub backend launch; not used as a percentile |
| Real Gemma smoke run | 1 | 29.4 s total | — | Whole script; listener ready after about 13 s; returned `ready` |
| Real Qwen TTS | 1 | 14.472 s | — | 56,534-byte WAV through `/v1/audio/speech` |

Master process snapshots around 40 direct plus 40 routed requests changed from 15,814,656 to 19,324,928 bytes working set, 182 to 221 handles, and 11 to 16 threads. These two snapshots do not prove a leak or a peak. No pprof endpoint was enabled, so goroutine counts were not collected. Sustained throughput, continuous peak working set, and recovery percentiles remain verification gaps; the report makes no scalability conclusion from isolated timings.

Raw ignored artifacts are under `.tmp/audit/`: lab configurations, logs, deterministic source/binary, audit executables, SQLite state, coverage profile, and the TTS WAV. Credentials are fixed test-only values. Local absolute paths are omitted from this report.

## Confirmed findings

### TRA-2026-001 — Executable updates proceed without authenticated digests

**Remediation status:** Resolved in code; default TUF publication activation is pending. Direct URLs without SHA-256 are rejected before network access, repository payloads require valid signed metadata, and failed refreshes preserve the current cache.

| Field | Value |
| --- | --- |
| Severity | High |
| Confidence | High |
| Subsystem | Update supply chain |
| Affected version | Audited revision; direct and repository update sources when updates are enabled and no digest is supplied |
| References | `internal/update/manager.go`: `validateTarget`, `download`; `internal/update/release_download.go`: `downloadReleasePayloads`; `internal/config/config.go`: `validateBackendUpdateSource`; empty digest fields in `config.example.yaml` and deploy configs |

Expected behavior: bytes that will become an executable must be authenticated by a required digest or signed release provenance before installation.

Observed behavior: `validateTarget` returns success with a nil expected hash when the SHA-256 field is empty. Both direct and release downloads log `SECURITY WARNING` and continue. Configuration validates digest syntax only “when provided,” and shipped examples leave every update digest empty. HTTPS protects transport to the selected endpoint but does not protect against publisher, release-account, CDN, or metadata compromise.

Minimal reproduction:

```powershell
rg -n "SECURITY WARNING|validateTarget|strings.TrimSpace\(target.SHA256\)" internal/update
rg -n "binary_sha256: \"\"" config.example.yaml deploy/config
go test -buildvcs=false ./internal/config ./internal/update
```

Impact: enabling the documented update mechanism can replace the router or native backend with attacker-controlled executable content after a source-side compromise. Updates are disabled by default, which reduces exposure but does not make the enabled path safe.

Root cause: authenticity is modeled as optional metadata, and warning-only behavior is explicitly implemented as a supported state.

Fix proposal: reject any executable payload lacking a verified digest. For repository releases, require a publisher-provided digest asset whose own provenance is authenticated, or verify a supported signature/attestation. Do not substitute a hash computed after downloading from the same unauthenticated release channel. Make the failure explicit in capability/status output.

Compatibility: existing users who enable updates with blank hashes will receive a startup/update error and must pin a digest or opt into a newly designed signed source. This is an intentional secure-breaking change.

Regression design and verification criteria:

- Direct URL with no digest is rejected before an HTTP request.
- Repository release with no authenticated digest is rejected before installation.
- Matching digest installs; mismatch leaves the current executable and metadata untouched.
- Logs never describe an unauthenticated executable installation as successful.

### TRA-2026-002 — Shipped LAN deployment exposes administrative mutation without authentication

**Remediation status:** Resolved. Shipped profiles are secure, credentials support mutually exclusive value/file sources with role separation, Compose uses file secrets, Portainer requires direct secrets, and trusted LAN is an explicit overlay.

| Field | Value |
| --- | --- |
| Severity | High |
| Confidence | High |
| Subsystem | Authentication and deployment defaults |
| Affected version | Shipped Compose/Portainer node and WebUI configurations |
| References | `deploy/config/node.yaml`, `router-managed.yaml`, `webui.yaml`; `deploy/compose.base.yaml`; `internal/auth/auth.go`: `Policy.Middleware`; `internal/webui/server.go`: `authenticationRequired` |

Expected behavior: a network-published administration surface requires a separate admin credential and CSRF/session enforcement by default.

Observed behavior: shipped configurations select `trusted_lan`, bind router and WebUI services to `0.0.0.0`, publish ports 8080/8443/8444, allow RFC1918 ranges, and configure empty inference/admin credentials. Router middleware deliberately skips inference and admin bearer checks in `trusted_lan`; only cluster routes retain mandatory token checks. The WebUI likewise skips session and CSRF enforcement in this profile. Administrative paths include load/unload, cook/config mutation, downloads, and router shutdown.

Minimal reproduction:

```powershell
rg -n "trusted_lan|0.0.0.0|inference_keys: \[\]|admin_keys: \[\]" deploy/config
go test -buildvcs=false ./internal/auth ./internal/webui -run TrustedLAN
```

Impact: any host on an allowed LAN can consume inference resources and invoke state-changing administrative operations. On shared office, VPN, Wi-Fi, or container-host networks, “LAN” is not an authentication boundary.

Root cause: a convenience trust profile is used as the published deployment default, and network exposure is combined with intentionally disabled application authentication.

Fix proposal: ship `secure` deployment profiles, require distinct inference and admin secrets from environment/secret files, keep the router bound to loopback or an internal container network behind the WebUI/reverse proxy, and make `trusted_lan` a conspicuous explicit opt-in overlay. Refuse wildcard secure binds with empty required credentials.

Compatibility: unattended example deployments must provision secrets. Existing explicitly selected `trusted_lan` behavior can remain available.

Regression design and verification criteria:

- Fresh Compose/Portainer deployment rejects unauthenticated inference and admin requests.
- An inference key cannot call admin routes.
- State-changing WebUI calls require an authorized session and valid CSRF token.
- Cluster token remains separate and is never accepted as inference/admin authorization.

### TRA-2026-003 — Duplicate cluster identities silently replace live nodes

**Remediation status:** Resolved. Registry mutation rejects local-ID/local-URL claims and duplicate ID/URL ownership without changing revision or catalog; registration returns typed HTTP 409 conflicts and rejected URLs are not route-authorized.

| Field | Value |
| --- | --- |
| Severity | High |
| Confidence | High |
| Subsystem | Cluster registry and registration |
| Affected version | Master clusters with duplicate/misconfigured node IDs or conflicting URL identity |
| References | `internal/cluster/registry.go:46`, `Registry.UpdateNode`; `internal/proxy/router_handlers.go`, `handleNodeRegister` |

Expected behavior: the master rejects a slave whose identity conflicts with the local master, an existing node at another URL, or an existing URL under another node ID.

Observed behavior: `UpdateNode` validates only that `NodeID` is non-empty and then assigns `registry.nodes[snapshot.NodeID] = ...`. A second snapshot with the same ID silently replaces the incumbent. A slave may also claim the master's local ID. Because route locality is decided using node ID, that collision can misclassify a remote route as local.

Minimal reproduction:

```powershell
go run -buildvcs=false .\.tmp\audit\cluster_evidence.go
```

Sanitized evidence:

```text
first_update=<nil>
second_update=<nil>
node_urls=map[duplicate:http://second master:http://master] models=[second-model]
```

Impact: accidental duplicate IDs silently remove capacity and redirect the public catalog to the last writer. A holder of the cluster credential and access to a configured slave URL can also exploit identity ambiguity to disrupt or confuse routing.

Root cause: the registry uses node ID as an overwrite key but has no immutable ID-to-normalized-URL ownership checks and no local-ID exclusion.

Fix proposal: normalize identities and reject `NodeID == localID`; reject an existing node ID with a different URL; reject an existing normalized URL with a different node ID; preserve the incumbent on conflict; return HTTP 409 for registration conflicts. Define an explicit administrative replacement/reset operation if identity migration is needed.

Compatibility: configurations that currently reuse IDs will stop registering and expose the misconfiguration immediately.

Regression design and verification criteria:

- Duplicate ID/different URL, duplicate URL/different ID, and local-ID claims return conflict without changing registry revision or catalog.
- Same ID/same normalized URL refreshes normally after restart.
- Concurrent conflicting registrations produce one owner deterministically.

### TRA-2026-004 — Cluster JSON calls have no total timeout and serialize health checks

**Remediation status:** Resolved. Buffered control calls retain the configured total deadline, slave probes are concurrently bounded and deterministically applied, shutdown cancels probes, and terminal duplicate registration propagates to runtime shutdown.

| Field | Value |
| --- | --- |
| Severity | High |
| Confidence | High |
| Subsystem | Cluster client, synchronization, health recovery |
| Affected version | Master and slave cluster control-plane JSON requests |
| References | `internal/cluster/client.go:34`, `Client.JSON`; `internal/cluster/sync.go:35`, `syncSlavesLoop` and `syncSlave` |

Expected behavior: an unresponsive node is marked unhealthy within a configured bound without delaying checks of other nodes.

Observed behavior: `NewClient` configures a 30-second `http.Client.Timeout`, but `Client.JSON` clones the client and sets `Timeout = 0`. Synchronization passes the long-lived service context without a per-call deadline and checks slave URLs sequentially. A server that accepts the connection and never completes a JSON response therefore blocks that sync loop indefinitely.

Minimal reproduction:

```powershell
go run -buildvcs=false .\.tmp\audit\cluster_evidence.go
```

Sanitized evidence:

```text
background_request_still_blocked_after=31s
```

The two-slave lab additionally observed unstable failover after one slave was killed: the surviving node initially served traffic, followed by 502 responses. That observation motivates the priority but is not required to prove the unbounded blocking defect.

Impact: one stalled slave can prevent stale-node transitions and recovery updates for every later slave in the configured list. Registration, load, unload, and other JSON control calls can also outlive the intended client bound and delay shutdown.

Root cause: streaming timeout behavior was applied to buffered JSON calls, and the sync topology has head-of-line blocking.

Fix proposal: retain a bounded total timeout for `JSON`, or derive a per-call context deadline from an explicit cluster control timeout. Keep unbounded body streaming only for APIs that need it. Probe configured slaves independently with bounded concurrency, then apply results deterministically.

Compatibility: control operations exceeding the bound will now fail and retry rather than hang. Stream transfer APIs remain separately governed.

Regression design and verification criteria:

- A handler that sends headers and never completes times out within the configured bound.
- A hanging first slave does not delay a healthy second slave's refresh.
- Shutdown cancels all outstanding cluster control calls promptly.
- Recovered nodes re-enter selection within one health interval plus the request bound.

### TRA-2026-005 — Container downloader lacks the required `hf` executable

**Remediation status:** Resolved in code. The companion now owns native Hub planning and verified HTTP transfers through framed stdio, performs a bounded capability handshake, and ships in every release matrix archive without `hf`, Python, curl, or shell execution.

| Field | Value |
| --- | --- |
| Severity | High |
| Confidence | High |
| Subsystem | Container deployment and downloader companion |
| Affected version | `node` and `webui` Containerfile targets with downloader enabled |
| References | `Containerfile:27`; `internal/downloader/manager.go:53`, `NewManager`; `manager.go:76`, `Capability`; `manager.go:487`, child execution; deployment config `downloader.binary_location` |

Expected behavior: the shipped image either contains every runtime dependency needed for an advertised download or reports the capability unavailable before accepting jobs.

Observed behavior: the runtime image installs only CA certificates and timezone data, then copies the Go companion. `NewManager` defaults its actual download command to `hf`, and jobs use `exec.CommandContext` to invoke it. The image never installs that executable. `Capability` checks storage but not command existence/version, so startup can report a working downloader and every job then fails with executable-not-found.

Minimal reproduction:

```powershell
rg -n "apk add|tensor-router-downloader" Containerfile
rg -n 'command = "hf"|exec.CommandContext|func \(manager \*Manager\) Capability' internal/downloader/manager.go
```

Docker was absent locally, so an image invocation was not run; the final runtime package list and unconditional child command make the failure deterministic.

Impact: the main advertised model-download path is nonfunctional in the published container topology, and capability reporting directs users into a job that cannot start.

Root cause: build-time inclusion of the companion was mistaken for inclusion of its external CLI dependency; capability probing is incomplete.

Fix proposal: preferably implement the download protocol in the companion using the existing Hub client and verified HTTP transfers. Otherwise install a version-pinned, hash-verified `hf` CLI and its supported runtime. At startup, resolve the executable, run a bounded version probe, and set `working=false` with a precise reason on failure.

Compatibility: image size changes if the external CLI is retained. A native implementation avoids Python/runtime drift.

Regression design and verification criteria:

- Container smoke test creates a small public-repository job and verifies its hash and atomic promotion.
- Removing the command makes capability false and causes job creation to fail before persistence as runnable work.
- Gated-token isolation and telemetry-disabled environment tests remain intact.

### TRA-2026-006 — systemd stop timeout truncates the configured drain

**Remediation status:** Resolved. The installer strictly validates a configurable deadline and defaults to 16m30s (990 seconds), exceeding the 15-minute application drain.

| Field | Value |
| --- | --- |
| Severity | Medium |
| Confidence | High |
| Subsystem | Linux service deployment and graceful shutdown |
| Affected version | Services installed with `scripts/install-systemd-user.sh` and default/example drain settings |
| References | `scripts/install-systemd-user.sh:44`; `config.example.yaml:123`; `deploy/config/node.yaml:108`; `deploy/config/router-managed.yaml:108` |

Expected behavior: the service manager allows at least the router's maximum graceful-drain duration plus cleanup margin.

Observed behavior: the router examples configure `drain_timeout: 15m`; Compose correctly grants 16 minutes, but the systemd installer sets `TimeoutStopSec=90s`, `SendSIGKILL=yes`, and `KillMode=mixed`.

Minimal reproduction:

```sh
grep -n 'TimeoutStopSec' scripts/install-systemd-user.sh
grep -n 'drain_timeout' config.example.yaml deploy/config/*.yaml
```

Impact: requests legitimately draining for more than 90 seconds are force-killed, and the backend process tree may not complete orderly cleanup.

Root cause: service timeout and application timeout are duplicated constants with inconsistent values.

Fix proposal: parameterize the installer stop timeout and default it to at least 16 minutes, or generate it from validated configuration plus a margin. Document that deployments must keep the service timeout greater than the application drain timeout.

Regression design and verification criteria: add a script-level generated-unit test and a Linux integration test with a request exceeding 90 seconds; SIGTERM must drain it and exit before systemd's deadline without SIGKILL.

### TRA-2026-007 — Executable replacement removes the working target before rename succeeds

**Remediation status:** Resolved in code. Executables are staged on the target filesystem, promoted with backup through Unix rename or Windows ReplaceFileW, verified before metadata commit, and rolled back on post-promotion failure.

| Field | Value |
| --- | --- |
| Severity | Medium |
| Confidence | High |
| Subsystem | Update installation and rollback |
| Affected version | Update installation on all platforms; especially Windows rename/error cases |
| References | `internal/update/manager.go:897`, `replaceBinary` |

Expected behavior: failure to promote a fully verified replacement leaves the previous executable available and executable.

Observed behavior: `replaceBinary` calls `os.Remove(targetPath)` before `os.Rename(tempPath, targetPath)`. If rename fails because of permissions, antivirus locking, cross-volume behavior, or another filesystem error, cleanup removes the temporary payload and the old target is already gone.

Minimal reproduction:

```powershell
Get-Content internal/update/manager.go | Select-Object -Skip 896 -First 18
go test -buildvcs=false ./internal/update
```

Impact: a transient installation failure can leave the router/backend executable missing, turning an update error into an outage requiring manual repair.

Root cause: replacement has no same-directory staging, backup, atomic swap, or rollback transaction.

Fix proposal: stage the verified file in the target directory, fsync as supported, preserve the prior target under a bounded backup name, use platform-specific atomic replacement, and restore on any promotion failure. Update metadata only after the target is successfully verified in place.

Regression design and verification criteria: inject rename failure and verify old bytes, permissions, and metadata remain unchanged; successful replacement must expose only old or new complete bytes to concurrent readers and clean its backup after verification.

## Inconclusive candidates and verification gaps

| Candidate | Evidence | Classification and next check |
| --- | --- | --- |
| Downloader SQLite pool pragmas | `internal/downloader/store.go` enables WAL/foreign keys once but has no busy timeout or connection cap; `Jobs` can need another connection while rows are open | Inconclusive. Reproduce with concurrent jobs and forced contention before changing pool behavior; a one-connection fix requires refactoring nested queries to avoid deadlock |
| Model-assets SQLite connection policy | Busy timeout and foreign keys are configured, but no explicit connection cap | Inconclusive. Existing mutex and tests did not expose contention; run a sustained concurrent resolver test with connection instrumentation |
| Downloader close waits | Baseline `Manager.Close` canceled contexts without joining job goroutines | Resolved during remediation. Manager cancellation now joins job goroutines before store/log closure, and companion close uses a bounded kill-and-reap path with blocking-child regressions |
| Multi-node failover instability | Reproduced 502/no-replica behavior in two failure runs while the surviving process remained alive | Inconclusive as a separate defect because the exact health-state transition was not captured after each request; rerun after TRA-2026-004 instrumentation/fix with registry state events |
| Race detector | Compiler absent and CGO disabled | Verification gap; run the exact CI/Linux command above |
| Container permissions and Compose schema | Static inspection completed; Docker/Podman absent | Verification gap; build CPU image, run as UID 10001 against fresh bind mounts, validate ownership and shutdown |
| SD 1.5 inference | Required model absent; insufficient safe disk | Verification gap; provide at least checkpoint size plus staging and runtime headroom, verify SHA-256 `6ce0161689b3853acaa03779ec93eafe75a02f4ced659bee03f50797806fa2fa`, then run direct and clustered requests |
| Split native saturation | llama and sd executables existed, Whisper server and SD model did not | Verification gap; use deterministic lane-specific native stubs or install all three supported servers and models |
| Sustained throughput/goroutine/peak-resource growth | Only sequential latency and before/after process snapshots were collected | Verification gap; enable a local diagnostic build or external sampler and retain raw time series |

## Rejected hypotheses

| Hypothesis | Classification | Evidence |
| --- | --- | --- |
| Downloader/update archive traversal or symlink escape | Rejected for audited paths | Destination validation rejects traversal/backslashes; portable schemas reject unsafe values; updater extraction validates relative containment and rejects unsafe link entries; focused tests passed |
| Peer asset promotion trusts partial bytes | Rejected | Final hash is verified before promotion; interrupted/resumed and concurrent single-transfer tests passed |
| Cluster token leaks through backend request headers or redirects | Rejected | Backend header allowlist strips credentials/forwarding metadata; cluster redirect code rejects unconfigured targets and tests verify no token leak |
| WebUI has an obvious unescaped dynamic HTML path | Rejected | Dynamic render paths consistently use escaping helpers; ESLint/TypeScript/Vitest passed; no exploitable sink/source pair was demonstrated |
| Public model-hash conflicts collapse into one model | Rejected | Registry conflict indexing assigns distinct public IDs; existing conflict and rewrite tests passed |
| Streaming retry replays a consumed request body | Rejected | Transport retries only before consumption; streaming/body-limit integration tests passed |
| Normal graceful shutdown leaves deterministic/Kobold child processes | Rejected in exercised paths | Clean three-node run and real Gemma/TTS runs left no matching child processes or listeners |
| SQLite stores universally omit busy handling | Rejected | Analytics and load-capture stores use single-connection policies and busy timeouts; model-assets sets a busy timeout; only the downloader-specific candidate remains inconclusive |

## Architectural observations tied to demonstrated risk

`internal/proxy/service.go` is approximately 84.8 KB and the proxy package has 54.7% statement coverage despite a large test suite. The file coordinates routing, loading, retry, streaming, cluster selection, analytics, recipes, and multiple backend lanes. This is not reported merely as a style concern: the multi-node failover state was difficult to observe at the point where registry health, reservation state, transport failure, and backend readiness intersected.

After the immediate defects, extract explicit lifecycle coordinators for cluster route acquisition, backend lane state, and drain/retry policy. Each coordinator should expose immutable snapshots or structured transition events usable by tests and operations. Preserve the existing small responsibility-specific proxy files and move behavior by dependency boundary, not by arbitrary file size.

## Original remediation sequence

The following sequence was the baseline remediation plan. Its current completion and remaining operational dependencies are recorded in the action table and implementation plan above.

### Immediate

1. Reject updater payloads without authenticated digests or signatures.
2. Change shipped deployments to `secure`, require separate secrets, and remove direct public router exposure where the WebUI/reverse proxy is present.
3. Restore bounded JSON timeouts and remove sequential health-check head-of-line blocking.
4. Reject duplicate/local cluster identities without mutating registry state.
5. Mark downloader capability unavailable until its actual command is present and proven runnable.

### Short term

1. Make executable installation transactional with rollback.
2. Align systemd stop timeout with the configured drain.
3. Add an end-to-end container downloader smoke test and a real two-slave failover/recovery CI test.
4. Reproduce downloader SQLite contention and close semantics before selecting a pool design.
5. Upgrade the indirect `golang.org/x/sys` dependency through the owning dependency graph and rerun the pinned vulnerability scan, even though the current advisory is not reachable.

### Architectural

1. Separate cluster health/control traffic from inference streaming clients and expose bounded timeout configuration.
2. Make node identity ownership explicit and persistent, with an administrative replacement workflow.
3. Reduce proxy lifecycle concentration around observable state machines and structured transition evidence.
4. Add opt-in diagnostic sampling for goroutines, handles/file descriptors, queues, and per-node health without exposing it outside the admin boundary.

## Ollama compatibility review

The router now treats Ollama as a distinct protocol surface rather than an OpenAI-shaped alias. `POST` is required for `/api/show`, `/api/generate`, `/api/chat`, and `/api/embed`; `GET` is required for `/api/tags`, `/api/ps`, and `/api/version`; mismatches return 405 with the exact `Allow` value. Authentication, request-limit, routing, backend, and mid-stream failures use Ollama's flat error envelope. Error translation buffers at most 64 KiB, and each NDJSON record is independently bounded to 4 MiB before model rewriting.

`/api/show` preserves `verbose` and validates contract-shaped fields. `/api/tags` uses deduplicated router-visible text models. `/api/ps` includes only loaded models on healthy nodes. Both expose the public ID in `name` and `model`, stable SHA-256 digests, UTC timestamps, indexed sizes when known, and available details. Regression tests cover method mismatches, error envelopes, oversized requests and backend errors, multi-record streams, missing final newlines, mid-stream errors, replica collapse, unhealthy exclusion, and local-ID leakage.

## Post-remediation validation

| Check | Result | Evidence |
| --- | --- | --- |
| `go test -buildvcs=false -count=1 -timeout 180s ./...` | Pass | Every Go package and command; includes updater, publication, downloader, cluster, proxy, auth, and WebUI server tests |
| `go vet -buildvcs=false ./...` | Pass | No diagnostics |
| High-risk suites, three repetitions | Pass | cluster, downloader, update, proxy, credential, atomicfile, and TUF publisher |
| WebUI `npm run check` | Pass | 0 advisories; ESLint; 16 files/46 Vitest tests; TypeScript; Vite production build |
| `govulncheck@v1.6.0 ./...` | Pass | No reachable vulnerabilities; 0 called vulnerable symbols |
| TUF adversarial tests | Pass | Invalid signature/expiry, rollback, mix-and-match, cache immutability, wrong keys, version monotonicity, HTTPS downgrade rejection, and clean-cache publication verification |
| GitHub Advanced Security | Pass | Code scanning on PR #2 reports no new alerts after malicious-content-type Ollama regressions cleared both reflected-XSS findings |
| Deployment/workflow parsing | Pass | 18 YAML files and Portainer/TUF JSON policies parsed |
| Cross-builds | Pass | All three executables for Linux amd64, Linux arm64, Linux ARMv7, and Windows arm64 |
| Release archive smoke | Pass | Linux amd64/arm64/ARMv7 archives each contain router, WebUI, and downloader |
| Credential/systemd helpers | Pass | Secret uniqueness/permissions/idempotence and generated `TimeoutStopSec=990s` validated; shell syntax passes |
| Race suite | Pass in CI | GitHub run `31183274035` passes the targeted cluster/downloader/update/proxy race suite on the merged revision; the Windows workstation still lacks `gcc` |
| Container/Compose runtime smoke | Pass in CI | GitHub run `31183274035` builds node and WebUI images, validates secure deployment manifests, and verifies image boundaries |
| Default TUF service | Pass | Publication run `31183299695`; Pages metadata and targets are public by design; a clean embedded-root client verified all eight signed manifests; exact initial public commit audit found zero private-key matches |
| Private TUF key custody | Restricted | All eight workstation key files moved outside the checkout to a non-inheriting current-user-only directory; root/targets keys are absent from GitHub and must be copied to separate offline media for disaster recovery |
| Built-in Linux arm64 backend updates | Unsupported upstream matrix | Current upstream releases do not provide the complete required asset set; the updater fails closed before payload download |

## Acceptance assessment

- Every original repository subsystem remains represented in the coverage matrix, and the remediation implementation plan is recorded above.
- TRA-2026-001 through TRA-2026-007 are closed with focused regressions, and the default TUF repository is active with protected online roles and independent live clean-client verification.
- Secure deployment defaults, credential role isolation, cluster identity ownership, bounded cluster control, self-contained downloader execution, transactional promotion, and Ollama protocol behavior are implemented and locally verified.
- Reachable Go and npm vulnerability scans pass after upgrading `golang.org/x/crypto` to v0.52.0 and `golang.org/x/sys` to v0.45.0. `GO-2026-5932` remains an informational module-level notice for the unmaintained `x/crypto/openpgp` package; that package is not imported, no vulnerable symbol is called, and no fixed module version exists.
- Race, container, and live public-TUF checks pass in GitHub or against the public endpoint. The local Windows host still lacks a C compiler and container runtime, and Linux arm64 built-in backend publication remains unsupported by the current upstream asset matrix.
- The original real-backend text and voice evidence remains valid for the audited baseline. SD image inference and full split-native live inference were not rerun and retain their documented host-resource limitations.

The remediation is code-complete and operationally active with fail-closed update behavior. The remaining actions are physical offline backup of the already separated root/targets custody files and future expansion of the upstream asset matrix; neither weakens current signed publication or the serving router.
