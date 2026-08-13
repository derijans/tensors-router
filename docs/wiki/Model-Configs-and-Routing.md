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

`router_unload_policy` in `.kcpps` controls which active runtimes are unloaded before a different configuration loads. Accepted values are:

- `none`
- `text`
- `image`
- `embeddings`
- `voice`
- `music`
- `all`

Current runtimes map image to the image backend. Text, embeddings, voice, and music map to the text backend. `all` targets every current lane.

The load API can supply an unload policy. The unload API accepts a target and uses `all` when no body is provided.

## Jinja kwargs profiles

**Load-time effect:** Use profiles with the same model and the same top-level `jinja_kwargs` keys for variants such as thinking enabled and disabled. The router can reuse the loaded runtime and model weights when only those values differ. Switching profiles then avoids the normal unload, backend restart, model load, and readiness wait, which significantly reduces the delay between compatible model variants.

For `/v1/chat/completions`, `jinja_kwargs` may be a JSON object or a string containing a JSON object. The router merges those values into the request's `chat_template_kwargs`.

`router_jinja_kwargs_precedence` accepts:

- `config`, which is the default and makes configured values win
- `client`, which makes client values win

A null client value is treated as absent. When configured kwargs are active, duplicate or non-object `chat_template_kwargs` fields are rejected.

Compatible profiles can share one loaded runtime. Compatibility requires all non-Jinja configuration values and the sorted top-level Jinja key set to match. A different key set or another runtime setting causes a normal reload.

For example, these values are compatible when every other `.kcpps` field is identical:

```json
{"enable_thinking": true}
```

```json
{"enable_thinking": false}
```

Both profiles contain the same `enable_thinking` key. The router applies the selected value through request `chat_template_kwargs` without reloading the model. Adding a key to only one profile makes them incompatible.

This behavior requires backend support for request-level `chat_template_kwargs`.

## Sharing configurations

Normal `.kcpps` files contain machine-local asset paths. Export a portable configuration before sharing it with another machine or cluster node. Portable files use asset hashes, safe filenames, and optional commit-pinned Hugging Face origins instead of local paths.

See [KCPPS Sharing](KCPPS-Sharing) for the export, resolution, and verification process.

## Asset hashes and public IDs

The cluster registry records configuration hashes, referenced model hashes, capabilities, backend family, node identity, and availability.

Nodes with the same model ID and identical hashes share one public ID. A conflicting model with different hashes receives an indexed public ID. Requests are mapped back to the selected node's local ID.

See [Cluster Routing](Cluster-Routing) for registration, health, and recipe behavior.
