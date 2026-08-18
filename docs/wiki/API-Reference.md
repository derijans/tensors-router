# API Reference

The router selects a `.kcpps` configuration and forwards recognized inference requests to its backend. An endpoint is usable only when the selected backend binary implements it and the selected configuration contains the required model components. Route recognition by the router does not add an API that is missing from KoboldCpp, llama.cpp, stable-diffusion.cpp, whisper.cpp, or vLLM.

Requests and responses retain streaming behavior. Server-sent event responses remain event streams.

## Model discovery

| Method and path | Result |
| --- | --- |
| `GET /v1/models` | User-facing text model IDs. |
| `GET /sdapi/v1/sd-models` | User-facing image model IDs. |
| `GET /router/v1/models` | Local and clustered records with capabilities, hashes, source, backend family, node, availability, and benchmark data when present. |

Cluster node IDs are not exposed through `/v1/models`. Image-only and combined configurations follow the availability rules in [Model Configs and Routing](Model-Configs-and-Routing).

A model disabled from the Models tab (or `POST /router/v1/site/models/state`) is omitted from listings and also rejected on inference: a request naming a disabled model returns the same HTTP 404 `model %q was not found` response as an unknown model ID, rather than loading and serving it.

## Text APIs

For KoboldCpp and split native backends, the router recognizes paths at `/v1` and `/v1/...` as text-side API paths unless a more specific image or voice classifier applies. vLLM routes use a strict method and path allowlist. Common backend APIs in this namespace include:

- `POST /v1/chat/completions`
- `POST /v1/completions`
- `POST /v1/responses`
- `POST /v1/responses/input_tokens`
- `POST /v1/messages`
- `POST /v1/messages/count_tokens`
- `POST /v1/rerank`
- `POST /v1/reranking`

The exact `/v1/...` operations depend on the selected text backend. The router also recognizes Kobold-compatible text paths:

- `/api/v1/generate`
- `/api/extra/generate/stream`
- `/api/extra/tokencount`

Ollama compatibility uses the official methods:

- `POST /api/show`
- `POST /api/generate`
- `POST /api/chat`
- `POST /api/embed`
- `GET /api/tags`
- `GET /api/ps`
- `GET /api/version`

Method mismatches return `405` with `Allow`. Ollama failures use `{"error":"message"}`. Generate and chat streams are NDJSON; each successful record exposes the router-visible model ID, while error-only records pass through unchanged. `/api/tags` is synthesized from deduplicated router-visible text models, and `/api/ps` contains only loaded models on healthy nodes. Backend-local model IDs are not exposed by either response.

The model-aware compatibility paths use the request model. Non-image KoboldCpp paths outside these groups are not proxied.

## vLLM serving APIs

The `vllm` backend allowlists stable online-serving operations for OpenAI Completions, Chat, Chat Batch, Responses, Embeddings, Transcriptions, Translations, Realtime transcription, Anthropic Messages and token counting, Cohere Embed and Rerank, Classification, Score, Pooling, Generative Scoring, SageMaker invocation, tokenize, and detokenize. Request and response bodies, multipart fields, streaming, and extra upstream parameters pass through subject to router limits and model-name rewriting.

`GET`, `DELETE`, and cancel operations below `/v1/responses/{id}` follow the node and runtime that created the response. `/v1/realtime` holds its model lease for the complete WebSocket connection, including cluster forwarding.

Operational vLLM paths are separate and administrator-authenticated below `/router/v1/vllm/...`. Only health, version, load, metrics, tokenizer information, and configured dynamic LoRA or Elastic Expert Parallelism operations are reachable. Unknown operations and development, profiler, RPC, weight-transfer, sleep, offline, gRPC, Ray, native multi-node, and disaggregated surfaces are not proxied.

Example:

```sh
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"hermes-8k","messages":[{"role":"user","content":"hello"}],"stream":true}'
```

## Embedding APIs

The router classifies these paths as embedding requests:

- `POST /v1/embeddings`
- `POST /api/embed`
- `POST /api/extra/embeddings`

The selected configuration must advertise an embedding component, and the selected backend must implement the requested compatibility form.

## Image and video APIs

The router recognizes these route groups:

- `/v1/images/...`
- `/sdapi/v1/...`
- `/sdcpp/v1/...`
- `/prompt`, `/queue`, `/history`, `/view`, `/object_info`, `/system_stats`, and `/interrupt`
- `/history/...`, `/view/...`, `/object_info/...`, and `/upload/image...`

The router itself provides model selection behavior for `/sdapi/v1/options`, refresh handling at `/sdapi/v1/refresh-checkpoints`, and discovery at `/sdapi/v1/sd-models`. Discovery requests for LoRAs, upscalers, schedulers, and stable-diffusion.cpp capabilities use the active image runtime when the selected backend provides them.

ComfyUI-style paths are classified as image requests. They are not implemented by `sd-server` in the split llama.cpp and stable-diffusion.cpp backend. Stable-diffusion.cpp asynchronous submissions under `/sdcpp/v1/...` retain their selected route for later job polling.

## Voice APIs

The router recognizes:

- `POST /v1/audio/speech`
- `POST /v1/audio/transcriptions`
- `POST /v1/audio/translations`
- `GET /v1/audio/voices`
- `GET /audio/voices`
- `POST /api/extra/tts`
- `POST /api/extra/transcribe`

Availability depends on the selected backend and `.kcpps` voice fields. In split mode, compatible TTS uses the llama runtime and configurations containing `whispermodel` use the independent Whisper runtime for all three STT routes. A TTS-only `ttsmodel` can be llama-server's primary model; a standalone `ttsmodel` cannot coexist with a text primary model. Supplemental `talkermodel` and `code2wavmodel` configurations remain valid.

OpenAI transcription and translation accept multipart WAV input and `json`, `verbose_json`, `text`, `srt`, or `vtt` output. Translation is forced to English by route. Kobold transcription accepts JSON `audio_data`, `prompt`, `langcode` or `language`, and `suppress_non_speech`, returning `{"text":"..."}`. ffmpeg conversion is not enabled; non-WAV input returns HTTP 400.

The model selector can be supplied in the multipart field, query, or `X-Tensors-Model` header. STT requests without a selector use automatic loaded/idle/queue-aware cluster scheduling described in [Cluster Routing](Cluster-Routing).

## Music APIs

The router classifies `/musicui`, `/musicui/...`, and `/api/extra/music/...` as music requests. They require backend support and the music components referenced by the selected `.kcpps` configuration.

## Explicit model control

Load a model:

```sh
curl http://127.0.0.1:8080/router/v1/load \
  -H "Content-Type: application/json" \
  -d '{"model":"hermes-8k"}'
```

Unload all active runtimes:

```sh
curl -X POST http://127.0.0.1:8080/router/v1/unload
```

Unload one target:

```sh
curl -X POST http://127.0.0.1:8080/router/v1/unload \
  -H "Content-Type: application/json" \
  -d '{"target":"image"}'
```

The target accepts `text`, `image`, `embeddings`, `voice`, `music`, or `all`.

`POST /router/v1/shutdown` is available in trusted-LAN mode or when secure mode has an admin key configured.

## Management APIs

Standalone and master routers expose administration routes in these groups:

- `/router/v1/load`, `/router/v1/unload`, and `/router/v1/shutdown`
- `/router/v1/models`
- `/router/v1/benchmarks` and `/router/v1/benchmarks/run`
- `/router/v1/site/inventory`
- `/router/v1/site/nodes/backends/init` and `/router/v1/site/nodes/backends/init/cancel`
- `/router/v1/site/download/...`
- `/router/v1/site/webuis/...`
- `/router/v1/site/analytics`
- `/router/v1/site/load-captures`
- `/router/v1/site/cook/...` and `/router/v1/site/config-file/...`
- `/router/v1/site/model-files/...` and `/router/v1/site/model-assets/...`

Load-capture recording is opt-in on each node. The site endpoint merges enabled nodes and accepts repeated `node` parameters plus `status`, `kind`, `backend`, `from`, `to`, `limit`, and `cursor` filters. Summary records expose content hashes instead of local model paths.

Detailed records and paged output are available from:

- `GET /router/v1/site/load-captures/{attempt_id}?node_id={node_id}`
- `GET /router/v1/site/load-captures/{attempt_id}/output?node_id={node_id}&after_sequence={sequence}`

Details contain the sanitized KCPPS snapshot and asset hashes. Output payloads are base64-encoded JSON byte fields, preserve stdout/stderr ordering, and may be marked truncated when the configured capture limit is reached. Site routes require admin authentication; corresponding `/router/v1/node/load-captures/...` routes require the cluster token.

The WebUI catalog, session toggle, model load, and proxied browser routes are described in [Backend WebUI Interfaces](Backend-WebUI-Interfaces).

Slave nodes expose cluster-authenticated routes under `/router/v1/node/...` for registration, inference forwarding, model loading, inventory, configuration, assets, downloads, benchmarks, analytics, and backend WebUI access. These are internal coordination interfaces used by the master and `tensor-router-webui`, not stable inference APIs for normal clients.

## Authentication

- Inference keys authorize model APIs.
- Admin keys authorize site APIs, benchmarks, explicit load and unload, and shutdown.
- The cluster token authorizes only `/router/v1/node/...` routes.

Send credentials as bearer tokens. See [Security and Reverse Proxy](Security-and-Reverse-Proxy) for profiles and network restrictions.
