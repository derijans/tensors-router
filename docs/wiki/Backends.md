# Backends

The router supports two backend families. `backend.mode` selects the default. A `.kcpps` file can set `backend_mode` to select a different family for that model.

Valid values are `kobold` and `llama_sdcpp`.

## KoboldCpp

`kobold` mode manages one [KoboldCpp](https://github.com/LostRuins/koboldcpp) process.

At router startup, the process starts in no-model mode when `kobold.no_model` is enabled. Model selection reloads the complete `.kcpps` file through the backend administration interface. Text, image, embedding, voice, and music requests share that process and its model gate.

A configuration switch waits for active requests using the current configuration to finish. Requests that use the same active configuration can run together.

KoboldCpp must provide:

- administration mode with configuration reload and unload support
- the requested text, image, embedding, voice, or music endpoint
- `chat_template_kwargs` support when Jinja kwargs profiles are used

The router removes the backend's router-mode argument because model selection is handled by `tensors-router`.

## llama.cpp and stable-diffusion.cpp

`llama_sdcpp` mode manages two independent processes:

- [llama.cpp](https://github.com/ggml-org/llama.cpp) `llama-server` for text, embeddings, multimodal input, and supported audio routes
- [stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp) `sd-server` for image and video routes

Both processes start lazily. Selecting a text model does not start the image process, and selecting an image model does not start the text process. Each lane waits only for its own in-flight requests during a switch unless an unload policy targets both lanes.

The selected binaries must expose the endpoints requested by clients. `sd-server` does not implement the ComfyUI queue and history endpoints recognized by the router.

## `.kcpps` mapping

The router reads the same `.kcpps` files in both backend families.

For KoboldCpp, it passes the complete configuration to the administration endpoint.

For `llama-server`, it maps text model paths, multimodal projector paths, context and batch sizes, thread counts, GPU offload, device splitting, memory settings, KV cache types, and supported vision limits to server arguments.

For `sd-server`, it maps diffusion model components, VAE and text encoder paths, LoRA settings, thread counts, device selection, offload settings, quantization, tiling, sampling, scheduler, and supported video settings to server arguments.

Backend-specific `extra_args` are appended after mapped arguments. Use them only for options that are accepted by the selected executable. Host and port overrides are rejected because managed backends must stay on their configured loopback listeners.

See [Cook Backend Options](Cook-Backend-Options) for the editable option catalog.

## Downloads and updates

Run the configured backend download operation with:

```sh
./tensors-router download --config config.yaml
```

Kobold mode resolves the KoboldCpp update source. Split mode resolves both llama.cpp and stable-diffusion.cpp sources.

Each source can use a direct HTTPS binary URL or a GitHub repository URL with an asset glob. Direct binaries, `.zip`, `.tar.gz`, and `.tgz` payloads are supported. Archive contents are extracted into the backend folder, so `binary_path` must identify the executable inside the extracted layout.

When a checksum is configured, it is verified before installation. For an archive, the checksum covers the downloaded archive.

Do not treat a newer upstream release as a tested compatibility statement. Check required features against the selected backend binary and keep deployment-specific sources in `config.yaml`.

## Endpoint differences

Both families support OpenAI-compatible text routing when the selected backend provides the requested endpoint.

KoboldCpp also handles router-recognized Stable Diffusion and ComfyUI-style paths through its single process.

Split mode sends `/v1/images/...`, `/sdapi/v1/...`, and `/sdcpp/v1/...` to `sd-server`. ComfyUI-style paths remain classified as image requests but fail if the configured backend does not implement them.
