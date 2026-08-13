# vLLM Backend Implementation Plan

1. Add vLLM runtime contracts: backend mode, typed `.kcpps` metadata, immutable snapshot identity, signed runtime manifests, profile compatibility, persistent initialization jobs, and atomic promotion.
2. Add resident `tensor-router-vllm` framed-protocol companion. Keep Python and vLLM execution isolated from router process and Go dependencies.
3. Integrate generation, pooling, and speech runtimes into existing load, drain, lease, cluster, analytics, diagnostics, and benchmark machinery.
4. Expose only allowlisted stable inference and administrator operations. Preserve bodies and streams, strip credentials and hop-by-hop headers, and route private runtime traffic through Unix sockets.
5. Add local and cluster backend initialization controls plus lifecycle-rich node state.
6. Add WebUI backend selection, Nodes initialization state/action/progress/cancellation, inventory, analytics, load-capture, benchmark, and validation coverage.
7. Add supported release archives, glibc container targets, accelerator overlays, signed runtime profile inputs, examples, and documentation without changing unsupported archives or Alpine images.
8. Run focused tests, full Go tests, race tests, WebUI checks, audits, vulnerability scans available locally, and security boundary review.

Implementation remains split by responsibility to avoid megafiles and overlapping edits.

## Remainder

9. Fix GitHub CodeQL Zip Slip finding in signed smoke-model extraction with filesystem-enforced destination containment and adversarial archive tests.
10. Add an offline targets-delegation rotation ceremony for existing TUF repositories. Produce a canonical unsigned payload, require the configured offline threshold signatures, verify the completed metadata, and never expose signing keys to CI or the online publisher.
11. Bind hardware profile receipts to exact manifest digests and protected runner identity. Publish only profiles whose installation, import, serving, Python audit, and runtime/container scans passed on matching hardware.
12. Reconcile every original acceptance item against tests and release workflows. Separate code-complete items from external signing and hardware evidence required before profile publication.
13. Re-run CodeQL-equivalent security tests, full Go/race/WebUI/audit gates, push the fixes, and update PR #8.
