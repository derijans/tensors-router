# Trusted Updates

Repository-based backend updates are authorized by The Update Framework (TUF). The router embeds an initial threshold-signed root and refreshes timestamp, snapshot, targets, and delegated target metadata before it downloads an executable. The signed target manifest fixes the upstream repository, release identifier, tag, payload URL, byte length, and SHA-256 digest. A missing, expired, rolled-back, mismatched, or invalid signature stops only the update; the installed backend remains available.

Direct executable URLs do not use repository discovery. Every direct URL requires its matching `updates.*_binary_sha256` value. There is no warning-and-continue mode.

## Trust boundary

Automated publication verifies each selected upstream release asset by downloading and hashing it independently before producing a target manifest. That process still trusts the configured upstream publisher account to choose the release bytes. The current upstream projects do not provide artifact attestations that establish a narrower build-system trust boundary.

## Runtime configuration

`updates.tuf_repository_url` identifies the HTTPS metadata directory and must end in `/metadata`. Queries, fragments, and embedded credentials are rejected. The associated targets directory is the sibling `/targets` path.

The default empty `updates.tuf_root_path` uses the root embedded at build time. A custom TUF service requires an explicit root file whose keys authorize that service. Replacing the root path is a trust-anchor change and should be deployed through the same reviewed channel as the router configuration.

The shipped publication policy currently covers Windows amd64 and Linux amd64 CPU releases. A runtime/backend combination without one exact configured upstream asset is unsupported by the built-in repository and fails before payload download. In particular, the complete `llama_sdcpp` backend set cannot be published for Linux arm64 while stable-diffusion.cpp provides no matching release asset. Operators may add a platform only after every required upstream publishes an asset and the configured glob is verified to match exactly one current release asset.

Trusted metadata is refreshed in a staging directory. It becomes current only after all signatures, versions, expirations, target hashes, target lengths, the installed executable, and executable permissions have passed verification. Failed refreshes are discarded without replacing the current cache.

vLLM runtime manifests use the same TUF trust model but are consumed only by `tensor-router-vllm` after an explicit administrator initialization action. A signed profile pins the Python, vLLM, plugin, wheel, source, or OCI artifacts with exact URLs, sizes, hashes, installation methods, supported platforms and devices, and prerequisites. The publication pipeline rejects nightlies, unresolved `latest` versions, and profiles that have not passed installation, import, serving, Python dependency audit, and runtime vulnerability gates on matching hardware or an approved vendor runner. Runtime profile staging never changes the current promoted environment until all verification and smoke tests succeed.

The `upstream-targets` delegation authorizes only `upstreams/*/*` and `runtimes/vllm/*`. Existing repositories created before vLLM support require an offline-authorized top-level targets rotation that adds `runtimes/vllm/*`; the online publication key cannot widen its own delegation. Publication rejects every target outside those paths and verifies clean-cache retrieval before timestamp metadata is written.

Runtime profiles are not sourced from `tuf/upstreams.json`. Approved hardware or vendor-runner receipts enter reviewed bundles below `tuf/profile-evidence`; see its README for the handoff contract. Dispatch `Validate vLLM runtime profiles` for that bundle, then dispatch trusted publication with the successful validation run ID. Publication verifies that the run used `.github/workflows/vllm-profile-evidence.yml` in this repository, succeeded on the exact publication commit, and uploaded an artifact named `vllm-runtime-profile-evidence` containing `evidence.json` plus `manifests/linux-amd64.json`, `manifests/linux-arm64.json`, and `manifests/darwin-arm64.json`. The evidence format is defined by [`tuf/runtime-profile-evidence.schema.json`](https://github.com/derijans/tensors-router/blob/main/tuf/runtime-profile-evidence.schema.json).

Evidence expires after 14 days. Every profile operating-system, architecture, and device combination must identify its hardware or vendor runner and successful installation, import, serving, Python dependency audit, and runtime vulnerability scan. OCI profiles also require a successful container scan; non-OCI profiles may mark that check `not_applicable`. Scheduled publication without new evidence preserves currently trusted runtime manifests rather than deleting or replacing them.

## Publication roles

Maintain root keys offline with a threshold greater than one. Root metadata authorizes top-level targets, snapshot, and timestamp. Top-level targets metadata authorizes the online delegated upstream-targets role. The online publication identity may update only upstream target manifests; it must not hold enough root keys to rotate or recover the repository.

Use short expirations for timestamp metadata, longer expirations for snapshot and delegated targets metadata, and a substantially longer but monitored root expiration. Publication monitoring must alert before any role reaches its renewal window.

## Scheduled publication

Run `go run ./cmd/tensor-router-tuf-bootstrap` once from a protected operator workstation. It writes private keys and an initial public repository under the ignored `.tmp/tuf-bootstrap` directory and replaces the embedded file with public root metadata only. Move the three root keys and two top-level targets keys into separate offline custody before provisioning anything online. Initialize the `tuf-metadata` branch with `.tmp/tuf-bootstrap/repository` under `tuf/` and add `.nojekyll` at the branch root. This layout serves the configured endpoint at `/tensors-router/tuf/metadata` through GitHub Pages.

Provision the base64 contents of `upstream-targets-1.ed25519.base64`, `snapshot-1.ed25519.base64`, and `timestamp-1.ed25519.base64` as environment-protected GitHub secrets with the corresponding `TUF_` names used by `publish-tuf.yml`. The publisher derives each public key and refuses any secret not authorized by the current root or targets metadata. Delete workstation copies only after offline custody, online provisioning, initial publication, and clean-client verification have all succeeded.

Each publication run must perform these operations in order:

1. Query the allowlisted upstream repositories for eligible releases and resolve immutable asset URLs.
2. Download every selected payload without using a digest supplied by the release API as the sole verification source.
3. Record the exact repository, release ID, tag, asset URL, byte length, and independently computed SHA-256 in the platform manifest under `upstreams/<backend>/<os>-<arch>.json`.
4. Increment delegated targets and snapshot versions, renew their expirations, and sign them with the online role.
5. Increment and sign timestamp metadata only after the snapshot and all referenced target files are ready.
6. Publish versioned metadata and targets first, then publish timestamp metadata last.
7. Verify the published repository from a clean cache using the embedded public root before declaring the run successful.

The online signing key belongs in an environment-protected CI secret with restricted maintainers and reviewed workflow changes. A publication job must never print private key material, tokens, signed download URLs, or secret-bearing request headers.

## Rotation and revocation

For a routine online-key rotation, create delegated metadata that authorizes the replacement public key, sign it with the currently authorized threshold, publish it, verify clean-client refresh, and only then remove the old private key.

For root rotation, produce each intermediate root version and sign it with both the threshold from the previous root and the threshold in the new root. Publish every intermediate version; clients must be able to advance one root version at a time. Update the embedded root in a router release after the public rotation chain is available.

If an online key is suspected compromised, stop publication, revoke it with offline-authorized metadata, increment every affected metadata version, publish a replacement timestamp last, and verify from both an existing cache and a clean cache. Do not reuse a revoked key.

If metadata expires before renewal, leave it expired until a correctly versioned replacement can be signed. Do not extend local clocks, lower versions, bypass signature checks, or replace cached metadata by hand. Existing backends continue serving while updates remain blocked.

## Recovery checks

Before restoring publication, verify offline backups can satisfy the configured root threshold, compare public key IDs against the embedded root and current repository, and rehearse the complete rotation on an isolated repository. After recovery, confirm signature, expiration, rollback, mix-and-match, length, and digest failures against clean and previously updated clients.
