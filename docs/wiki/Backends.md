# Backends

The router supports three backend families. `backend.mode` selects the default. A `.kcpps` file can set `backend_mode` to select a different family for that model.

Valid values are `kobold`, `llama_sdcpp`, and `vllm`.

## KoboldCpp

`kobold` mode manages one [KoboldCpp](https://github.com/LostRuins/koboldcpp) process.

At router startup, the process starts in no-model mode when `kobold.no_model` is enabled. Model selection reloads the complete `.kcpps` file through the backend administration interface. Text, image, embedding, voice, and music requests share that process and its model gate.

Configs that run separately (see [Separate runtimes](#separate-runtimes)) are the exception: their requests lazily start a second KoboldCpp process on a router-allocated loopback port. An embeddings-lane config is hosted `--nomodel` with a private role-specific runtime configuration written below the model configuration directory, so the primary process never receives embedding fields and the embedding process receives no unrelated model components.

## Separate runtimes

Any kobold or `llama_sdcpp` config can be marked **Separate** in the WebUI (per node, stored in the model-state database; `.kcpps` files are not rewritten). A config marked separate, or a legacy embeddings config with `run_embed_separate: true`, runs in its own backend process from a pooled set, with its own model gate. Another config's load, switch, or unload on the shared runtime never touches it.

- The pool is capped by `limits.separate_runtimes` (default 5). When it is full, the least-recently-used entry is unloaded and its port returned to the allocator, including an entry whose triggers say "do not unload".
- Each separate config carries its own `router_unload_policy` trigger set, which decides which loads elsewhere evict it. `none` means no trigger evicts it (it can still be evicted by pool pressure). Pool runtimes are never evicted by another config's `router_unload_policy`.
- Placement (CPU vs GPU) stays whatever the `.kcpps` says; the toggle only decides process isolation.
- Each entry appears in the Nodes tab as a `<mode>-separate-<id>` runtime and is individually unloadable.

`kobold.embeddings_backend_url` and `llama.embeddings_backend_url` are deprecated: an embeddings config now joins this pool on a router-allocated port. A pinned value still loads but logs a deprecation warning.

A configuration switch waits for active requests using the current configuration to finish. Requests that use the same active configuration can run together.

KoboldCpp must provide:

- administration mode with configuration reload and unload support
- the requested text, image, embedding, voice, or music endpoint
- `chat_template_kwargs` support when Jinja kwargs profiles are used

The router removes the backend's router-mode argument because model selection is handled by `tensors-router`.

## Native split backends

`llama_sdcpp` mode manages three independent processes:

- [llama.cpp](https://github.com/ggml-org/llama.cpp) `llama-server` for text, embeddings, and multimodal input
- [stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp) `sd-server` for image and video routes
- [whisper.cpp](https://github.com/ggml-org/whisper.cpp) `whisper-server` for transcription and translation

For `llama_sdcpp`, `run_embed_separate: true` (or the WebUI Separate toggle) sends embedding requests to an on-demand `llama-server` from the [separate-runtime pool](#separate-runtimes) on a router-allocated port. CPU configurations force `--device none` and zero GPU layers; GPU configurations fully offload the embedding model and inherit configured device placement.

All processes start lazily and drain independently. Transcription uses the Whisper runtime. Text-to-speech is not available under `llama_sdcpp`: llama.cpp removed `--model-vocoder` and `--model-talker`, and llama-server has no `/v1/audio/speech` endpoint. Use `kobold` or `vllm` for text-to-speech.

The selected binaries must expose the endpoints requested by clients. `sd-server` does not implement the ComfyUI queue and history endpoints recognized by the router; the router answers those itself for video-producing workflows only, using `sd-server`'s or KoboldCpp's native video generation underneath. See [API Reference](API-Reference).

`sd-server`'s and KoboldCpp's video output is never MP4: `sd-server` emits WebM, animated WebP, or MJPG-AVI, and KoboldCpp emits GIF or MJPG-AVI. The router's ComfyUI video emulation transcodes every finished job to H.264/AAC MP4 with ffmpeg, which must be reachable via `ffmpeg.binary_path` or `PATH`. ffmpeg is also used to convert non-WAV transcription input on the buffered whisper request path. Missing ffmpeg is not a startup failure; it fails only the requests that need it. See [Deployment](Deployment).

## vLLM companion backend

`vllm` mode uses three lazy runtimes for generation, pooling, and speech. A separate resident `tensor-router-vllm` companion manages signed runtime initialization, isolated Python environments, vLLM processes, health, logs, restart, and unload. Python and vLLM never enter the router executable or Go dependency graph.

Initialization occurs only after an explicit administrator action. Model loading remains offline and cannot install packages or download snapshots. See [vLLM](vLLM) for supported platforms, profiles, `.kcpps` fields, endpoint boundaries, and release gates.

## `.kcpps` mapping

The router reads `.kcpps` files for every backend family.

For KoboldCpp, it passes the complete configuration to the administration endpoint.

For `llama-server`, it maps text model paths, multimodal projector paths, context and batch sizes, thread counts, GPU offload, device splitting, memory settings, KV cache types, and supported vision limits to server arguments.

For `sd-server`, it maps diffusion model components, VAE and text encoder paths, LoRA settings, thread counts, device selection, offload settings, quantization, tiling, sampling, scheduler, and supported video settings to server arguments.

For `whisper-server`, shared fields map the model, threads, device, flash attention, and CPU controls. Native options use the `whispercpp_*` prefix. Existing llama-only unprefixed fields remain aliases; canonical `llama_*` fields win when both are present.

Backend-specific `extra_args` are appended after mapped arguments. Use them only for options that are accepted by the selected executable. Host and port overrides are rejected because managed backends must stay on their router-assigned loopback listeners, whether that address is pinned by configuration or allocated by the router at startup.

See [Cook Backend Options](Cook-Backend-Options) for the editable option catalog.

## MCP

An active embedded MCP configuration is passed to `llama-server` with `--mcp-servers-config`.

For KoboldCpp, the router creates a private `.router-mcp/<config>.kcpps` overlay inside the admin configuration jail and reloads the real configuration with that overlay as its base configuration.

Neither backend receives an MCP path embedded in the source `.kcpps`.

## Downloads and updates

Run the configured backend download operation with:

```sh
./tensors-router download --config config.yaml
```

Kobold mode resolves the KoboldCpp update source. Split mode resolves llama.cpp, stable-diffusion.cpp, and whisper.cpp sources.

Each source can use a direct HTTPS binary URL or a GitHub repository URL with an asset glob. Direct binaries, `.zip`, `.tar.gz`, and `.tgz` payloads are supported. Archive contents are extracted into the backend folder, so `binary_path` must identify the executable inside the extracted layout.

When a checksum is configured, it is verified before installation. For an archive, the checksum covers the downloaded archive.

Do not treat a newer upstream release as a tested compatibility statement. Check required features against the selected backend binary and keep deployment-specific sources in `config.yaml`.

## Endpoint differences

Both families support OpenAI-compatible text routing when the selected backend provides the requested endpoint.

KoboldCpp also handles router-recognized Stable Diffusion and ComfyUI-style paths through its single process.

Split mode sends `/v1/images/...`, `/sdapi/v1/...`, and `/sdcpp/v1/...` to `sd-server`. ComfyUI-style paths remain classified as image requests but fail if the configured backend does not implement them.

Split mode adapts `/v1/audio/transcriptions`, `/v1/audio/translations`, and `/api/extra/transcribe` to Whisper's `/inference` endpoint. Input is WAV-only. See [Whisper.cpp](Whisper.cpp).
