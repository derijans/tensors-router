# Backend Release Baselines

Audit completed: 2026-11-16. Every release published since the 2026-09-24 audit was checked: KoboldCpp v1.122.1 through v1.124.2, stable-diffusion.cpp master-909 through master-1106, llama.cpp b11158 through b12441, whisper.cpp v1.9.5, and vLLM v0.31.0 and v0.32.0. vLLM `v0.33.0rc3` has no final tag and was not adopted.

| Backend | Source | Verified marker |
| --- | --- | --- |
| KoboldCpp | https://github.com/LostRuins/koboldcpp/releases | `v1.124.2` |
| stable-diffusion.cpp | https://github.com/leejet/stable-diffusion.cpp/releases | `master-1106-532d2c1` |
| llama.cpp | https://github.com/ggml-org/llama.cpp/releases | `b12441` |
| whisper.cpp | https://github.com/ggml-org/whisper.cpp/releases/tag/v1.9.5 | `v1.9.5` |
| vLLM | https://github.com/vllm-project/vllm/releases | `v0.32.0` |

The listed markers are compatibility baselines for this router release, not a promise that they remain upstream's newest tags. whisper.cpp publishes Windows archives for v1.9.5 under the matching nightly tag `b5454`.

KoboldCpp `v1.124.2` and llama.cpp `b12441` were exercised end to end by the router smoke runs on Windows CPU builds. stable-diffusion.cpp `master-1106-532d2c1` was exercised for routing only, because the smoke setup has no image model; its new launch flags, tile size units, sound-to-video audio and upscale endpoint are covered by unit tests against the upstream source. whisper-server is unchanged between v1.9.4 and v1.9.5. vLLM `v0.32.0` has no Windows companion build and was verified against its release notes and source only.

## Launch flag requirements

- **llama.cpp model loading** uses `--load-mode` (`auto`, `none`, `mmap`, `mlock`, `mmap+mlock`, `dio`), available since `b10105`. llama.cpp `b10875` removed `--mlock`, `--mmap`, `--no-mmap` and `--direct-io`, so the router no longer emits them. `.kcpps` files that only set `usemmap` and `usemlock` are translated to the matching `--load-mode` value.
- **llama.cpp speculative drafts** are selected with `--spec-type` (`draft-dflash`, `draft-dspark`, and the other draft and n-gram types). `draft_dflash` and `draft_dspark` map there, and the draft layer count maps to `--spec-draft-ngl`.
- **llama.cpp `--reasoning-preserve`** is an on/off flag pair. Reasoning preservation is on by default since `b10763`; setting `preserve_reasoning` through chat template kwargs is deprecated upstream.
- **stable-diffusion.cpp `--auto-fit`** takes `on` or `off` and is on by default since `master-845-80bac2d`. `--stream-layers` was removed in `master-843-462d675`; `sdstreamlayers` and `sdstreaming` are kept as legacy keys and no longer emitted.
- **stable-diffusion.cpp flag names** are matched exactly. The router emits `--llm_vision`, `--clip_vision`, `--embeddings-connectors`, `--hires-upscalers-dir`, `--circularx` and `--circulary`.
- **whisper-server** has no `--no-context` flag; the server never carries context between requests. `whispercpp_no_context` is a legacy key.
- **KoboldCpp queue limit**: `v1.123.1` deprecated `--multiuser` and reads the queue limit only from `--queuelimit` (default 10). The router passes `kobold.multiuser` as `--queuelimit`, so KoboldCpp `v1.123.1` is the minimum. A `queuelimit` inside a loaded `.kcpps` wins over the launch value, because KoboldCpp applies config fields over its launch arguments when it reloads.
- **KoboldCpp agent mode**: from `v1.122.1`, `agent: true` opens an agent terminal and raises the context to at least 32768. The router rejects `agent` and `agentload` for `backend_mode: kobold`, in the cook and when it loads a `.kcpps`.
- **stable-diffusion.cpp compute modes**: `--flash-attn` (`master-962-d763dae`) and `--conv-direct` (`master-963-46362fc`) take `on`, `off`, `auto` or per-module assignments and default to `auto`. `--fa`, `--diffusion-fa`, `--diffusion-conv-direct` and `--vae-conv-direct` became deprecated shims. The router emits only the new flags: `sdflashattention` maps to `--flash-attn on`, `sddiffusionflashattention` to `off,diffusion=on`, `sddiffusionconvdirect` to `--conv-direct auto,diffusion=on,controlnet=on` and `sdvaeconvdirect` to `auto,vae=on`, the same values the shims set. `sdflashattentionmode` and `sdconvdirectmode` set the modes directly and win over the booleans. Stable-diffusion.cpp `master-963-46362fc` is the minimum for `llama_sdcpp` image configs that set any of these fields.
- **stable-diffusion.cpp `--batched-cfg`** (`master-956-1b0ba10`) is on by default and runs both CFG branches in one UNet pass; `sdbatchedcfg: false` turns it off.
- **stable-diffusion.cpp `--vae-tile-size`** is measured in image pixels since `master-919-19bbbca` (it was latent units before). The router's `sdtiledvae` choices (512, 768, 1024) were always pixel sizes, so they now mean what they say.
- **llama.cpp `--defrag-thold`** is gone from the `b12441` option list, and llama-server's default port is 9931 since `b11521`. The router never emitted `--defrag-thold` and always passes `--port`.

## Per-feature minimums

- **llama.cpp**:
  - `--n-cpu-ffn` (`n_cpu_ffn`) needs `b10645`.
  - `--video-fps`, `--video-timestamp-interval` and `--video-ffmpeg-dir` need `b10647`. OpenAI `video_url` content and `data:` video URIs need `b11136`. The router passes `--video-ffmpeg-dir` when its own ffmpeg probe succeeded and the config has a multimodal projector.
  - `--kv-unified-per-slot` needs `b10662`.
  - `--lazy-mode` (`lazy_mode`) needs `b10700`; it replaced `--tensor-read-lazy`.
  - The MoE expert GPU cache (`moe_cache_mib`, `moe_cache_cpu_share`) needs `b11480` and a GPU backend: on a CPU-only runtime llama-server stops at startup with `MoE cache requires a GPU backend`.
  - `--spec-draft-n-ubatch` (`spec_draft_n_ubatch`) needs `b11630`. The `draft-lilicorr` speculative type needs `b11641` and `draft-dflash2` needs `b11835`. `spec_draft_sampling` and `shard` were verified at `b12441`.
  - `--media-allow-private-urls` (`media_allow_private_urls`) needs `b12340`; from that build llama-server refuses media URLs that point to local or private addresses unless it is set.
  - Text-to-speech through `POST /v1/audio/speech` needs `b12238` (`POST /tts` exists since `b11781`). The router loads a `ttsmodel` with its codec mmproj in `ttswavtokenizer` into the llama-server text runtime. Verified with the llama.cpp conversion of Qwen3-TTS-12Hz-1.7B-Base; KoboldCpp's Qwen3-TTS GGUFs do not load in llama-server.
- **stable-diffusion.cpp**:
  - LTX-2.5 needs `master-833-afd5306`.
  - SenseNova U1.5 needs `master-857-7f986a9`.
  - Wan2.2 S2V (audio plus image to video) needs `master-863-0bd72f0` and `sdaudioencoder` (`--audio-encoder`). ComfyUI video requests with audio references need `master-1014-012c7f8`, where `POST /sdcpp/v1/vid_gen` accepts `ref_audios`; sd-server reads them only as WAV, so the router converts other formats with ffmpeg.
  - `POST /sdcpp/v1/upscale` needs `master-910-4dfe8f5`; the router runs it on the loaded image runtime.
  - `--max-pending-jobs` and `--completed-ttl-seconds` (`sdmaxpendingjobs`, `sdcompletedttlseconds`) need `master-1080-a3f46b6`.
  - `master-982-0518b34` added recipe-based model management (`/sdcpp/v1/models/load`, `/models/unload`, `/models/refresh`, `/models/status`, `/sdcpp/v1/model-files`, `/sdcpp/v1/recipes`). The router answers those paths with `404 not_found`, directly and through the sd-server WebUI proxy, because they would change the loaded model behind the router.
  - External tokenizers (`sdtokenizer`, `--tokenizer`) need `master-867-4964abd`. PiD and Lens models require one.
  - Qwen Image 2.1 (RGBA input and output, `qwen_image_2_1_prefix_cache` model argument) needs `master-883-137f740`.
  - LLaDA-Image (`llada_image` scheduler) needs `master-886-15f335d`.
  - SageAttention (`sdsageattention`, `--sage-attn`) needs `master-889-c678dfe`.
  - `image_preprocess` needs `master-899-28b454b`. It replaces the removed `auto_resize_ref_image` request field together with `ref_image_args` (`resize_before_vae=false`).
  - Conditioning cache (`sdconditioningcachesize`) needs `master-902-e6281b6`.
- **KoboldCpp**:
  - `usedirectio` needs `v1.120`.
  - `ffncpu`, `reasoningeffort: xhigh`, MiniMax-H3 media references (several reference images plus audio clips) and video LoRAs need `v1.121`.
  - `ubatchsize`, `sdllm` and `autoswapthreshold` need `v1.122.1`; `queuelimit` needs `v1.123.1`; `lazy_mode`, `moe_cache_mib` and `paralleltotalcontext` need `v1.124.2`.
  - `v1.121` renamed `GET /sdapi/v1/get_last.png` to `GET /sdapi/v1/get_last.json`, which returns the last generation as JSON. It also removed the `reverse_refimg` image field and added `video_start_frame` and `video_end_frame`.
  - From `v1.121`, image generation with `keepalive`, with `keep_image_gen_on_disconnect`, or behind Cloudflare (`CF-Connecting-IP`) answers with a close-delimited JSON body padded by whitespace lines. The router forwards the padding immediately.
- **whisper.cpp v1.9.4** enables token timestamps only for `verbose_json`, `max_len` or `split_on_word`, and its language detection response includes `language`. v1.9.5 ships the same whisper-server.
- **vLLM**:
  - `POST /v1/messages/render` and `POST /cohere/v2/chat/render` need `0.29.0`; `POST /v1/responses/render` needs `0.30.0`. From `0.30.0` the render endpoints, `/derender` and `/inference/v1/generate` are only registered when the model's `serve_args` include `--enable-scale-out`. The router proxies the three render endpoints and never `/derender` or `/inference/v1/generate`.
  - `0.29.0` deprecated `python -m vllm.entrypoints.openai.api_server`; the router launches `vllm serve` with one API server process.
  - `0.29.0` removed the Arctic, Chameleon, Cheers, Fairseq2Llama, FireRedLID, GritLM, HCXVision, MPT and PrithviGeoSpatialMAE architectures and the `RWForCausalLM` and `StableLMEpochForCausalLM` aliases.
  - `0.30.0` removed the `VLLM_PREFIX_CACHE_RETENTION_INTERVAL` and `VLLM_MM_HASHER_ALGORITHM` environment variables; `0.29.0` removed `VLLM_TEST_FORCE_FP8_MARLIN`. `VLLM_ROCM_USE_AITER_FP4_ASM_GEMM` is still present in `0.31.0` and `0.32.0`.
  - `--max-num-active-seqs` (`settings.max_num_active_seqs`) needs `0.31.0`.
  - From `0.31.0`, per-request `mm_processor_kwargs` and `media_io_kwargs` are rejected unless the server runs with `--trust-request-mm-kwargs`. The router sets that flag only when both `vllm.trust_request_mm_kwargs` in the router configuration and `trust_request_mm_kwargs` in the model configuration are true, and rejects it in `serve_args`.
  - `0.31.0` removed `tokenizer_mode="slow"`, the AllSpark INT8 W8A16 GEMM backend, `VLLM_PLE_CPU_OFFLOAD`, `VLLM_XPU_ENABLE_XPU_GRAPH` and the `return_assistant_tokens_mask` option of `/render`.
  - `0.32.0` needs Python 3.11 to 3.14, `transformers>=5.16.1,<5.20.0` and `tokenizers>=0.23.2`. It removed cascade attention, `mamba_cache_mode="all"` and the torch.compile sequence parallelism and AsyncTP passes, restricted the default CORS origins, stopped reserving `max_tokens` from the context window, and renamed the Responses API reasoning stream events. The router's unverified install defaults to Python 3.12.
- **MiniMax-H3 video generation** requires KoboldCpp `v1.119` or stable-diffusion.cpp `master-812-ea7f0c8` or newer. H3 is video-only: both backends reject it for still-image generation. `sdaudiovae` (`--audio-vae`) must be set for H3's audio track; KoboldCpp only preserves that audio in its MJPG-AVI video output, not its GIF output.
- **`minimax-01` GGUF architecture** (`MiniMaxText01ForCausalLM`, `MiniMaxM1ForCausalLM`) requires llama.cpp `b10437` or newer, or a KoboldCpp release built from it (`v1.119` or newer). vLLM removed both architectures in `0.23.0`.
- **`--model-vocoder` and `--model-talker` were removed from llama.cpp** on 2026-08-04; text-to-speech moved to mtmd, where a speech model loads with `--model` and its codec with `--mmproj`. The router never emits the removed flags and still rejects `talkermodel` and `code2wavmodel` under `backend_mode: llama_sdcpp`; `ttsmodel` with `ttswavtokenizer` is the llama.cpp text-to-speech config.
- **`POST /v1/images/edits`** and **`GET /sdapi/v1/progress`** are KoboldCpp endpoints since `v1.119`.
- KoboldCpp `v1.118.1` removed **Row Split** (`--splitmode row` on CUDA); `v1.117.1` was the last version supporting it, and the RPC protocol changed to match upstream llama.cpp's.

Virtual Jinja kwargs profiles require KoboldCpp v1.114.1 or newer and `chat_template_kwargs` support from the listed llama.cpp baseline. The listed KoboldCpp baseline satisfies that minimum.

## Known, not adopted

- llama.cpp `--host` with several addresses: the router binds each runtime to one loopback address.
- llama.cpp `--log-jsonl`: readiness and load error detection read the text log.
- llama.cpp `LLAMA_ARG_TEMPERATURE` and the other sampling environment variables: the router passes sampling settings on the command line.
- llama.cpp `--spec-synth-len` and `--spec-synth-rates`: benchmarking only.
- KoboldCpp `--analyze` for safetensors files, the reworked MusicUI, the Stable UI LoRA selector and the `KCPP_LORA` container variable: served or used by KoboldCpp itself.
- llama.cpp `/v1/realtime`, `/v1/systemone`, `--allow-unauthenticated-tools` (the router binds runtimes to loopback), `--connect`, `--connect-code`, `--models-memory-margin` and `--nextn`.
- stable-diffusion.cpp `--taesd-preview-only` and sd-server live previews: the router maps no `--taesd` and does not forward previews.
- KoboldCpp `rpccache`: KoboldCpp ignores it when it reloads a config, so a `.kcpps` value never takes effect under the router. `GET /api/extra/last_logprobs` is forwarded untouched.
- vLLM `/v1/systemone`, `/v1/decisions`, `/kv_event_sources` and the development-mode routes, and every `v0.33.0rc3` item.
- vLLM features are deferred to a later router release: admission control (`--max-num-queued-reqs`, `--max-num-queued-tokens`), per-request speculative decoding metrics, `reasoning_tokens` usage reporting, `--engram-config`, `--grpc`, and the new model families (DeepSeek-V4.1-Flash, GLM-5.3-Flash, K2-Horizon, Qwen3.8-Flash-Next). Flags without a dedicated `.kcpps` field can still be passed through `vllm.serve_args`.
