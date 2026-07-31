# Cook Backend Options

The Cook interface builds `.kcpps` configurations from the option catalog maintained in [`internal/cook/options.go`](https://github.com/derijans/tensors-router/blob/main/internal/cook/options.go).

That catalog is the source of truth for field names, input types, backend ownership, native argument mappings, defaults, choices, legacy status, and model-file roles. This page describes how the catalog is organized without duplicating every volatile backend choice.

## Editing behavior

- Every field accepts a custom value even when the interface provides choices.
- Unknown observed keys remain editable under Other.
- Missing, null, and empty values are ignored when comparison colors are calculated.
- Model path fields use node inventory filtered by the field's model role.
- Preview validates the selected node, backend family, model components, and option types before a configuration is written.
- Apply writes normal `.kcpps` files to one node or a master recipe when components span nodes.

## Router-owned fields

### `backend_mode`

Selects `kobold` or `llama_sdcpp` for the generated configuration. When omitted, the node's configured backend family is used.

### `router_unload_policy`

Accepts `none`, `text`, `image`, `embeddings`, `voice`, `music`, or `all`. The router applies the selected target before loading a different configuration.

### `router_jinja_kwargs_precedence`

Accepts `config` or `client`. It controls which value wins when configured `jinja_kwargs` are merged into request `chat_template_kwargs`.

## Runtime options

Runtime fields cover:

- configuration inheritance and side files
- managed host and port values
- launcher and administration behavior
- request concurrency and limits
- logging and debugging
- RPC and device selection
- llama.cpp model presets and autoload settings

The router owns managed backend host and port arguments. Cooked values cannot bypass the loopback validation applied when a backend manager is created.

## Text options

Text fields cover:

- model and draft-model paths
- CPU threads, batch sizes, and context size
- GPU layers, main device, and tensor splitting
- memory mapping, memory locking, and automatic fitting
- KV cache types and unified cache behavior
- speculative decoding and prompt caching
- LoRA files and multipliers
- Jinja templates, thinking behavior, and kwargs
- parallel request and continuous batching settings
- model metadata and tensor overrides

Some fields apply only to KoboldCpp or only to `llama-server`. The catalog records that backend ownership and the WebUI filters incompatible choices.

## MCP

`mcp_servers` is an embedded JSON array and `mcp_enabled` selects whether its generated backend artifact is active. Each entry has a unique nonempty `name` and an object `definition`. Stdio definitions use `command`, optional string-array `args`, and optional string-valued `env`. KoboldCpp also accepts HTTP(S) `url` definitions with string-valued `headers`. Legacy `mcpfile` paths are rejected.

The generated file is never a portable field. Literal environment values and headers in `mcp_servers` are portable by design, so export or share only configurations whose secrets are intended to travel.

## Multimodal and embedding options

These fields cover:

- multimodal projector paths and projector placement
- vision resolution and token limits
- embedding model paths and context size
- embedding GPU placement
- pooling behavior

The selected model files must be present on the node that will run the generated configuration.

## Image and video options

Image fields cover:

- diffusion, VAE, encoder, vision, ControlNet, PuLID, and upscaler files
- LoRA files and model directories
- image threads, device placement, offload, and VRAM limits
- quantization, tensor types, and tensor rules
- attention, convolution, tiling, circular padding, and streaming
- sampling methods, schedulers, RNG, prediction types, and cache modes
- stable-diffusion.cpp backend, parameter backend, and RPC settings

The catalog marks legacy options separately so existing configurations remain editable without presenting them as the preferred interface.

## Voice and music options

Voice fields cover transcription models, speech models, tokenizers, directories, thread counts, GPU placement, talker models, and waveform decoders.

Shared Whisper fields include `whispermodel`, `threads`, `maingpu`, `flashattention`, and compatible CPU controls. Native server options use `whispercpp_*`, including processors, timing and segment limits, decoding thresholds, language and prompt, translation and diarization, output controls, OpenVINO and DTW, suppression, language probabilities, and the complete VAD group. `whispercpp_vad_model` is a portable asset field.

Every llama-only option also has a canonical `llama_*` spelling. Existing unprefixed keys remain supported, and the canonical value takes precedence.

Music fields cover the language model, embedding model, diffusion model, VAE, and low-memory behavior.

Backend capabilities determine whether a cooked voice or music configuration can be loaded.

## Source references

- [KoboldCpp source](https://github.com/LostRuins/koboldcpp/blob/concedo/koboldcpp.py)
- [KoboldCpp Wiki](https://github.com/LostRuins/koboldcpp/wiki)
- [llama.cpp server documentation](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md)
- [stable-diffusion.cpp server documentation](https://github.com/leejet/stable-diffusion.cpp/blob/master/examples/server/README.md)
- [stable-diffusion.cpp CLI documentation](https://github.com/leejet/stable-diffusion.cpp/blob/master/examples/cli/README.md)
- [whisper.cpp server documentation](https://github.com/ggml-org/whisper.cpp/blob/master/examples/server/README.md)

When upstream introduces or removes a backend option, update the catalog and its focused tests first. The WebUI and this page should describe the resulting catalog rather than maintain a separate option list.
