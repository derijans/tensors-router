# vLLM profile evidence

This directory accepts reviewed manifests for matching hardware or approved vendor runners. It intentionally contains no release profiles, receipts, or placeholder artifacts.

A bundle may cover any subset of the supported platforms:

- `manifests/linux-amd64.json`
- `manifests/linux-arm64.json`
- `manifests/darwin-arm64.json`

At least one must be present, and only these names are accepted. Platforms a bundle omits keep whatever runtime manifest is already published for them; the publisher merges a new bundle with the existing `runtimes/vllm/` targets rather than replacing the whole set. This means a platform can be brought online on its own, as soon as a runner exists for it.

Run `Run vLLM hardware profile gate` once for every profile OS, architecture, and device tuple declared by the manifests in the bundle. Each successful run performs installation, import, serving, Python dependency audit, runtime scan, and OCI scan when applicable. It emits a short-lived Actions receipt only after every gate passes.

Runner classes:

- **Self-hosted.** Required for every accelerator profile (`cuda`, `rocm`, `xpu`, `metal`). The runner must carry the `self-hosted`, `vllm-<os>`, `vllm-<arch>`, and `vllm-<device>` labels, and the publication gate re-checks those labels against the run's own job record.
- **GitHub-hosted.** Accepted only for `device: cpu` profiles, because a hosted runner exposes no accelerator and cannot attest one. The receipt records which class produced it, and publication rejects a hosted receipt for any non-CPU device.

After all runs complete, dispatch `Validate vLLM runtime profiles` with this bundle path and their run IDs. It authenticates each run, commit, workflow, runner class, runner identity, and hardware labels before assembling `evidence.json` and producing the short-lived `vllm-runtime-profile-evidence` artifact accepted by trusted publication. No evidence is inferred from manifest claims or accepted from user-authored status strings.

Until a bundle is published, the router and companion fall back to the default manifest embedded in the release binary and report the `embedded-default` trust tier. Publishing a bundle here is what moves a platform to the `tuf` tier.
