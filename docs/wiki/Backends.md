# Backends

The router supports three backend families. `backend.mode` selects the default. A `.kcpps` file can set `backend_mode` to select a different family for that model.

Valid values are `kobold`, `llama_sdcpp`, and `vllm`.

## KoboldCpp

`kobold` mode manages one [KoboldCpp](https://github.com/LostRuins/koboldcpp) process.

At router startup, the process starts in no-model mode when `kobold.no_model` is enabled. Model selection reloads the complete `.kcpps` file through the backend administration interface. Text, image, embedding, voice, and music requests share that process and its model gate.

Configurations with `run_embed_separate: true` are the exception: embedding requests lazily start a second KoboldCpp process on `kobold.embeddings_backend_url`. The router writes private role-specific runtime configurations below the model configuration directory so the primary process never receives embedding fields and the embedding process receives no unrelated model components.

Standalone embeddings use one router-wide embedding slot. They can run beside any primary backend family, and switching text, image, or voice families leaves them running. Loading another embedding configuration replaces the current embedding owner. Replacing a shared-process embedding may unload that process's other lanes.

A configuration switch waits for active requests using the current configuration to finish. Requests that use the same active configuration can run together.

KoboldCpp must provide:

- administration mode with configuration reload and unload support
- the requested text, image, embedding, voice, or music endpoint
- `chat_template_kwargs` support when Jinja kwargs profiles are used

The router removes the backend's router-mode argument because model selection is handled by `tensors-router`.

## Native split backends

`llama_sdcpp` mode manages three independent processes:

- [llama.cpp](https://github.com/ggml-org/llama.cpp) `llama-server` for text, embeddings, multimodal input, and compatible text-to-speech
- [stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp) `sd-server` for image and video routes
- [whisper.cpp](https://github.com/ggml-org/whisper.cpp) `whisper-server` for transcription and translation

For `llama_sdcpp`, `run_embed_separate: true` sends embedding requests to an on-demand `llama-server` at `llama.embeddings_backend_url`. CPU configurations force `--device none` and zero GPU layers; GPU configurations fully offload the embedding model and inherit configured device placement.

All processes start lazily and drain independently. Speech uses the llama runtime, while transcription uses the Whisper runtime. A `voice` unload drains both; `all` drains every runtime.

The selected binaries must expose the endpoints requested by clients. `sd-server` does not implement the ComfyUI queue and history endpoints recognized by the router.

## vLLM companion backend

`vllm` mode uses three lazy runtimes for generation, pooling, and speech. A separate resident `tensor-router-vllm` companion manages signed runtime initialization, isolated Python environments, vLLM processes, health, logs, restart, and unload. Python and vLLM never enter the router executable or Go dependency graph.

Initialization occurs only after an explicit administrator action. Model loading remains offline and cannot install packages or download snapshots. See [vLLM](vLLM) for supported platforms, profiles, `.kcpps` fields, endpoint boundaries, and release gates.

## `.kcpps` mapping

The router reads `.kcpps` files for every backend family.

For KoboldCpp, it passes the complete configuration to the administration endpoint.

For `llama-server`, it maps text model paths, multimodal projector paths, context and batch sizes, thread counts, GPU offload, device splitting, memory settings, KV cache types, and supported vision limits to server arguments.

For `sd-server`, it maps diffusion model components, VAE and text encoder paths, LoRA settings, thread counts, device selection, offload settings, quantization, tiling, sampling, scheduler, and supported video settings to server arguments.

For `whisper-server`, shared fields map the model, threads, device, flash attention, and CPU controls. Native options use the `whispercpp_*` prefix. Existing llama-only unprefixed fields remain aliases; canonical `llama_*` fields win when both are present.

Backend-specific `extra_args` are appended after mapped arguments. Use them only for options that are accepted by the selected executable. Host and port overrides are rejected because managed backends must stay on their configured loopback listeners.

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
