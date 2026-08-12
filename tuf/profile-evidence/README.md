# vLLM profile evidence

This directory accepts reviewed manifests for matching hardware or approved vendor runners. It intentionally contains no release profiles, receipts, or placeholder artifacts.

Each bundle must contain:

- `manifests/linux-amd64.json`
- `manifests/linux-arm64.json`
- `manifests/darwin-arm64.json`

Run `Run vLLM hardware profile gate` once for every profile OS, architecture, and device tuple on a protected self-hosted or approved vendor runner. Each successful run performs installation, import, serving, Python dependency audit, runtime scan, and OCI scan when applicable. It emits a short-lived Actions receipt only after every gate passes.

After all runs complete, dispatch `Validate vLLM runtime profiles` with this bundle path and their run IDs. It authenticates each run, commit, workflow, protected runner identity, and hardware labels before assembling `evidence.json` and producing the short-lived `vllm-runtime-profile-evidence` artifact accepted by trusted publication. No evidence is inferred from manifest claims or accepted from user-authored status strings.
