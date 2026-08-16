# Release-Only CUDA and ROCm Container Images

## Implementation

- [x] Consolidate CPU, vLLM, CUDA, and ROCm targets in one pinned `Containerfile`.
- [x] Keep CPU images lightweight and keep Python, PyTorch, vLLM, and compiler SDKs outside production images.
- [x] Add native CUDA and ROCm node/WebUI targets for vendor runtime libraries.
- [x] Add vLLM CUDA and ROCm node/WebUI targets containing the companion and vendor runtime libraries.
- [x] Add self-contained CUDA, ROCm, vLLM-CUDA, and vLLM-ROCm Compose overlays.
- [x] Retain NVIDIA and AMD device-only overlays as deprecated compatibility paths.
- [x] Point accelerator Portainer templates at vendor images and add four vLLM accelerator templates.
- [x] Document image override variables for all twelve public tags.

## Release Workflow

- [x] Restrict push and pull-request container work to deployment-manifest validation.
- [x] Reject non-release events, non-published actions, prereleases, and tags outside `vMAJOR.MINOR.PATCH`.
- [x] Build and push digest-addressed candidate images only for accepted stable releases.
- [x] Verify every candidate boundary, linkage contract, and HIGH/CRITICAL fixed vulnerability scan before promotion.
- [x] Promote version and `latest` manifests only after the entire candidate matrix passes.
- [x] Publish CPU and CUDA images for `linux/amd64` and `linux/arm64`, and ROCm images for `linux/amd64`.

## Verification

- [x] Run local release-gate and static manifest checks.
- [x] Run repository tests and WebUI checks.
- [ ] Build, inspect, link-check, and scan the full image matrix in the stable release workflow.

The final image-matrix verification requires a published stable GitHub Release because ordinary CI intentionally does not generate images.
