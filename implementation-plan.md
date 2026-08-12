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
