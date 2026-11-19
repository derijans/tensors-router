# Model Configs and Routing

## Model discovery

The router reads `.kcpps` JSON files from `models.config_dir`. The file name stem is the base model ID.

Examples:

```text
kcpps/hermes-8k.kcpps -> hermes-8k
kcpps/sdxl.kcpps with sdmodel C:\models\juggernautXL.safetensors -> sdxl-juggernautXL
```

The router parses the fields needed for discovery, capability classification, backend selection, routing, and native argument mapping. KoboldCpp still receives the complete configuration.

## Configuration types

- Text configurations advertise an ID through `/v1/models`.
- Embedding configurations are selected by requests to embedding endpoints.
- `run_embed_separate` defaults to `false`. When `true`, embedding requests lazily start a dedicated managed backend while all other request types continue to use their existing runtimes.
- Standalone embedding selection is independent of primary backend selection. One standalone embedding runtime is active across KoboldCpp, llama.cpp, and vLLM; a later embedding load replaces it, while ordinary primary loads do not.
- Multimodal configurations use a text model with a projector.
- Image-only configurations advertise an image ID through `/sdapi/v1/sd-models`.
- Combined text and image configurations expose both capabilities.
- Voice configurations can contain transcription or speech components.
- Music configurations contain the model components required by the backend music endpoint.

Image IDs combine the configuration name and diffusion model file stem. A combined KoboldCpp image ID is advertised only while that combined configuration is active. Split-mode lanes can advertise the image component without loading the text component.

## Request selection

`/v1/chat/completions` and `/v1/completions` require a string `model` field. The router resolves it before forwarding the request.

Other recognized model endpoints select a configuration when their request contains a model ID. Requests without a model ID use the currently active compatible runtime when that route permits it.

Image selection can use:

- JSON `model`
- JSON `sd_model_checkpoint`
- JSON `override_settings.sd_model_checkpoint`
- query `model`
- query `sd_model_checkpoint`
- `X-Tensors-Model`

Voice and music routes use their documented request model fields. Cluster routing rewrites public IDs to node-local IDs before forwarding and rewrites response IDs back to the public value where required.

## Backend selection

`backend.mode` provides the default backend family. A `.kcpps` `backend_mode` value overrides that default for the model.

The selected model record includes its effective backend family, so a cluster master can route models hosted by nodes that use different backend families.

## Load and unload

An implicit model selection or `POST /router/v1/load` loads the complete configuration or the required split lane.

`router_unload_policy` in `.kcpps` controls which active runtimes are unloaded before a different configuration loads. It is a set: either one bare value (the legacy shape, still accepted) or a JSON array. Entries:

- `none`: do not unload anything. Absorbing: cannot be combined with other triggers.
- `all`: unload every lane. Absorbing.
- `text` · `image` · `embeddings` · `voice` · `music`: the capability lane.
- `family:kobold` · `family:llama_sdcpp` · `family:vllm`: any config loading on that backend family.
- `config:<model-id>`: one named sibling config.

Current runtimes map image to the image backend. Text, embeddings, voice, and music map to the text backend. `all` targets every current lane.

For a config that runs in the [separate-runtime pool](Backends#separate-runtimes), the same trigger set decides which loads on the shared runtime evict the pooled runtime. Pool runtimes are never evicted by another config's `router_unload_policy`.

The load API can supply an unload target. The unload API accepts one target or a list and uses `all` when no body is provided.

### Idle unload

`router_idle_unload_seconds` unloads a config after it has been idle for that many seconds. It is off by default: a missing key, `null`, or `0` keeps the config loaded until something else unloads it. Values are whole seconds from `0` to `604800` (7 days). Any other value disables idle unload for that config, and the router logs the reason once.

- Idle means no request holds the runtime, no request is waiting for it, and no load is in progress. Lent (borrowed) work counts as use.
- The router checks every 5 seconds, so an unload can happen up to 5 seconds after the delay ends.
- A runtime with a pending borrow restore is left alone until the restore finishes.
- It applies per runtime, including [separate-runtime pool](Backends#separate-runtimes) entries. A pool entry keeps its slot and port and reloads on the next request.
- Like other `router_*` keys, it is not part of the runtime-sharing comparison. Profile variants that differ only in this key share one runtime, and the delay of the config that is currently loaded applies.

### Memory fit

The Models tab has a **Fit** column that estimates, per node, whether each config fits that node's free memory. It is a warning only: a load is never blocked or delayed by it.

The exact footprint is known only after a load, so the router learns from its own loads:

- After a load into an empty runtime, with no other runtime loading or changing at the same time, the router stores the VRAM and RAM the load added. It keeps the newest 20 samples per combination of node, model file, backend, GPU layers, KV cache types, flash attention, tensor split and context size. A load that replaced another config is not used, because its before and after readings mix two models.
- **Measured**: the same combination was loaded before; the band is the lowest to highest stored sample.
- **Scaled from another context size**: the same model and placement at a different `contextsize`. With two or more measured sizes the router fits a line; with one it adds the KV cache difference read from the GGUF header. For sliding-window models the low end ignores that difference, because those layers may never allocate it.
- **Estimated from the same architecture**: no sample of this model, but samples of other models with the same `general.architecture` on this node; the band is wider.
- **Not learned**: nothing to go on, so no warning is shown.

Free memory counts what is free now plus what the load would release: the model held by the same lane runtime and the runtimes `router_unload_policy` would unload.

| Badge | GPU backends | RAM-only (CPU builds, `gpulayers` 0, Apple unified memory) |
|---|---|---|
| won't fit | even the low end of the estimate exceeds free VRAM plus free RAM | the low end exceeds free RAM |
| large spill | the high end overflows VRAM by more than `memory_fit.deep_spill_percent` (10%) of the need | not used |
| small spill | the high end overflows VRAM by less than that | not used |
| tight | less than `memory_fit.tight_headroom_mb` (512 MB) would remain | less than that would remain |
| fits | more room remains | more room remains |

"Won't fit" is decided from the low end of the estimate, so a config that might fit is never shown as red. Spill only works where the driver can fall back to system memory; elsewhere it ends in an out-of-memory error, and the tooltip says so. On a node with several GPUs the estimate compares against all GPUs together, so a bad `tensor_split` can still run out on one GPU.

`GET /router/v1/site/nodes/memory-fit?node_id=` returns the verdicts for one node. A master asks the node over `/router/v1/node/memory-fit`, and each node judges with its own memory readings.

## Jinja kwargs profiles

**Load-time effect:** Use profiles with the same model and the same top-level `jinja_kwargs` keys for variants such as thinking enabled and disabled. The router can reuse the loaded runtime and model weights when only those values differ. Switching profiles then avoids the normal unload, backend restart, model load, and readiness wait, which significantly reduces the delay between compatible model variants.

For `/v1/chat/completions` and the Ollama `/api/chat` and `/api/generate`, `jinja_kwargs` may be a JSON object or a string containing a JSON object. The router merges those values into the request's `chat_template_kwargs`. An Ollama `think` value only fills `enable_thinking` when the config does not set it.

`router_jinja_kwargs_precedence` accepts:

- `config`, which is the default and makes configured values win
- `client`, which makes client values win

A null client value is treated as absent. When configured kwargs are active, duplicate or non-object `chat_template_kwargs` fields are rejected.

Compatible profiles can share one loaded runtime. Compatibility requires all non-Jinja configuration values and the sorted top-level Jinja key set to match. With `models.share_runtime_by_launch_arguments` on (the default), `router_*` keys and the `gendefaults` the router injects (see [Generation defaults](#generation-defaults)) are left out of that comparison. A different key set or another runtime setting causes a normal reload.

For example, these values are compatible when every other `.kcpps` field is identical:

```json
{"enable_thinking": true}
```

```json
{"enable_thinking": false}
```

Both profiles contain the same `enable_thinking` key. The router applies the selected value through request `chat_template_kwargs` without reloading the model. Adding a key to only one profile makes them incompatible.

This behavior requires backend support for request-level `chat_template_kwargs`.

llama.cpp keeps reasoning from earlier assistant turns by default (`preserve_reasoning` on since `b10763`). Set `reasoning_preserve` in the `.kcpps` to change the default for the whole runtime, or put `preserve_reasoning` in a profile's `jinja_kwargs` to change it per request.

## Generation defaults

**Load-time effect:** Variants that differ only in `gendefaults` (for example an instruct and a thinking profile with different temperatures) share one loaded runtime, on both KoboldCpp and llama.cpp.

With `models.inject_request_defaults` on (the default), the router adds a config's `gendefaults` to each generation request instead of leaving them to KoboldCpp. It follows KoboldCpp's rule: a field the client sent is kept, and a field the client left out is filled in. With `gendefaultsoverwrite: true` the configured value always wins. `gendefaults` may be a JSON object or a string holding one; a value KoboldCpp could not parse is ignored, as KoboldCpp ignores it.

| Request | Where the defaults go |
| --- | --- |
| `/v1/chat/completions`, `/v1/completions`, `/v1/responses`, `/v1/messages`, `/api/v1/generate`, `/api/extra/generate/stream` | Top level of the body |
| `/api/chat` (Ollama) | Top level of the body on KoboldCpp, the `options` object elsewhere |
| `/api/generate` (Ollama) | The `options` object |

KoboldCpp receives the config's own field names. llama.cpp and the Ollama `options` object receive llama.cpp names: `rep_pen` and `repetition_penalty` become `repeat_penalty` (the largest value wins, as in KoboldCpp), `rep_pen_range` becomes `repeat_last_n`, and `typical` becomes `typical_p`. An explicit llama.cpp name in `gendefaults` wins over its KoboldCpp spelling. Fields llama.cpp does not know are passed on and ignored by it. vLLM configs get no injected defaults.

A config that also loads an image model keeps `gendefaults` inside KoboldCpp, because KoboldCpp also reads image settings from it. Embedding, rerank and image requests are never changed.

Requests from other clients that KoboldCpp used to fill from `gendefaults` on a route not listed above no longer get those defaults; set `models.inject_request_defaults: false` to keep KoboldCpp applying them. With `inject_request_defaults` on and `models.share_runtime_by_launch_arguments` off, the router still injects the defaults but every config change reloads the backend.

## Sharing configurations

Normal `.kcpps` files contain machine-local asset paths. Export a portable configuration before sharing it with another machine or cluster node. Portable files use asset hashes, safe filenames, and optional commit-pinned Hugging Face origins instead of local paths.

See [KCPPS Sharing](KCPPS-Sharing) for the export, resolution, and verification process.

## Asset hashes and public IDs

The cluster registry records configuration hashes, referenced model hashes, capabilities, backend family, node identity, and availability.

Nodes with the same model ID and identical hashes share one public ID. A conflicting model with different hashes receives an indexed public ID. Requests are mapped back to the selected node's local ID.

See [Cluster Routing](Cluster-Routing) for registration, health, and recipe behavior.
