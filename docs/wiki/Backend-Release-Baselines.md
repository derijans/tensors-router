# Backend Release Baselines

Audit completed: 2026-09-24. Every release published since the 2026-08-26 audit was checked: KoboldCpp v1.120 and v1.121, stable-diffusion.cpp master-832 through master-908, llama.cpp b10637 through b11157, whisper.cpp v1.9.4, and vLLM v0.29.0 and v0.30.0.

| Backend | Source | Verified marker |
| --- | --- | --- |
| KoboldCpp | https://github.com/LostRuins/koboldcpp/releases | `v1.121` |
| stable-diffusion.cpp | https://github.com/leejet/stable-diffusion.cpp/releases | `master-908-88411ef` |
| llama.cpp | https://github.com/ggml-org/llama.cpp/releases | `b11157` |
| whisper.cpp | https://github.com/ggml-org/whisper.cpp/releases/tag/v1.9.4 | `v1.9.4` |
| vLLM | https://github.com/vllm-project/vllm/releases | `v0.30.0` |

The listed markers are compatibility baselines for this router release, not a promise that they remain upstream's newest tags. whisper.cpp publishes Windows archives for v1.9.4 under the matching nightly tag `b5130`.

## Launch flag requirements

- **llama.cpp model loading** uses `--load-mode` (`auto`, `none`, `mmap`, `mlock`, `mmap+mlock`, `dio`), available since `b10105`. llama.cpp `b10875` removed `--mlock`, `--mmap`, `--no-mmap` and `--direct-io`, so the router no longer emits them. `.kcpps` files that only set `usemmap` and `usemlock` are translated to the matching `--load-mode` value.
- **llama.cpp speculative drafts** are selected with `--spec-type` (`draft-dflash`, `draft-dspark`, and the other draft and n-gram types). `draft_dflash` and `draft_dspark` map there, and the draft layer count maps to `--spec-draft-ngl`.
- **llama.cpp `--reasoning-preserve`** is an on/off flag pair. Reasoning preservation is on by default since `b10763`; setting `preserve_reasoning` through chat template kwargs is deprecated upstream.
- **stable-diffusion.cpp `--auto-fit`** takes `on` or `off` and is on by default since `master-845-80bac2d`. `--stream-layers` was removed in `master-843-462d675`; `sdstreamlayers` and `sdstreaming` are kept as legacy keys and no longer emitted.
- **stable-diffusion.cpp flag names** are matched exactly. The router emits `--llm_vision`, `--clip_vision`, `--embeddings-connectors`, `--hires-upscalers-dir`, `--circularx` and `--circulary`.
- **whisper-server** has no `--no-context` flag; the server never carries context between requests. `whispercpp_no_context` is a legacy key.

## Per-feature minimums

- **llama.cpp**:
  - `--n-cpu-ffn` (`n_cpu_ffn`) needs `b10645`.
  - `--video-fps`, `--video-timestamp-interval` and `--video-ffmpeg-dir` need `b10647`. OpenAI `video_url` content and `data:` video URIs need `b11136`. The router passes `--video-ffmpeg-dir` when its own ffmpeg probe succeeded and the config has a multimodal projector.
  - `--kv-unified-per-slot` needs `b10662`.
  - `--lazy-mode` (`lazy_mode`) needs `b10700`; it replaced `--tensor-read-lazy`.
- **stable-diffusion.cpp**:
  - LTX-2.5 needs `master-833-afd5306`.
  - SenseNova U1.5 needs `master-857-7f986a9`.
  - Wan2.2 S2V (audio plus image to video) needs `master-863-0bd72f0` and `sdaudioencoder` (`--audio-encoder`). sd-server has no HTTP audio input yet, so ComfyUI video requests with audio references are rejected on the `llama_sdcpp` backend.
  - External tokenizers (`sdtokenizer`, `--tokenizer`) need `master-867-4964abd`. PiD and Lens models require one.
  - Qwen Image 2.1 (RGBA input and output, `qwen_image_2_1_prefix_cache` model argument) needs `master-883-137f740`.
  - LLaDA-Image (`llada_image` scheduler) needs `master-886-15f335d`.
  - SageAttention (`sdsageattention`, `--sage-attn`) needs `master-889-c678dfe`.
  - `image_preprocess` needs `master-899-28b454b`. It replaces the removed `auto_resize_ref_image` request field together with `ref_image_args` (`resize_before_vae=false`).
  - Conditioning cache (`sdconditioningcachesize`) needs `master-902-e6281b6`.
- **KoboldCpp**:
  - `usedirectio` needs `v1.120`.
  - `ffncpu`, `reasoningeffort: xhigh`, MiniMax-H3 media references (several reference images plus audio clips) and video LoRAs need `v1.121`.
  - `v1.121` renamed `GET /sdapi/v1/get_last.png` to `GET /sdapi/v1/get_last.json`, which returns the last generation as JSON. It also removed the `reverse_refimg` image field and added `video_start_frame` and `video_end_frame`.
  - From `v1.121`, image generation with `keepalive`, with `keep_image_gen_on_disconnect`, or behind Cloudflare (`CF-Connecting-IP`) answers with a close-delimited JSON body padded by whitespace lines. The router forwards the padding immediately.
- **whisper.cpp v1.9.4** enables token timestamps only for `verbose_json`, `max_len` or `split_on_word`, and its language detection response includes `language`.
- **vLLM**:
  - `POST /v1/messages/render` and `POST /cohere/v2/chat/render` need `0.29.0`; `POST /v1/responses/render` needs `0.30.0`. From `0.30.0` the render endpoints, `/derender` and `/inference/v1/generate` are only registered when the model's `serve_args` include `--enable-scale-out`. The router proxies the three render endpoints and never `/derender` or `/inference/v1/generate`.
  - `0.29.0` deprecated `python -m vllm.entrypoints.openai.api_server`; the router launches `vllm serve` with one API server process.
  - `0.29.0` removed the Arctic, Chameleon, Cheers, Fairseq2Llama, FireRedLID, GritLM, HCXVision, MPT and PrithviGeoSpatialMAE architectures and the `RWForCausalLM` and `StableLMEpochForCausalLM` aliases.
  - `0.30.0` removed the `VLLM_PREFIX_CACHE_RETENTION_INTERVAL` and `VLLM_MM_HASHER_ALGORITHM` environment variables; `0.29.0` removed `VLLM_TEST_FORCE_FP8_MARLIN` and `VLLM_ROCM_USE_AITER_FP4_ASM_GEMM`.
- **MiniMax-H3 video generation** requires KoboldCpp `v1.119` or stable-diffusion.cpp `master-812-ea7f0c8` or newer. H3 is video-only: both backends reject it for still-image generation. `sdaudiovae` (`--audio-vae`) must be set for H3's audio track; KoboldCpp only preserves that audio in its MJPG-AVI video output, not its GIF output.
- **`minimax-01` GGUF architecture** (`MiniMaxText01ForCausalLM`, `MiniMaxM1ForCausalLM`) requires llama.cpp `b10437` or newer, or a KoboldCpp release built from it (`v1.119` or newer). vLLM removed both architectures in `0.23.0`.
- **`--model-vocoder` and `--model-talker` were removed from llama.cpp** on 2026-08-04 (TTS moved to the `llama-tts` CLI via mtmd). The router no longer emits either flag, and `.kcpps` fields that depended on them (`ttsmodel`, `ttswavtokenizer`, `talkermodel`, `code2wavmodel`) are rejected under `backend_mode: llama_sdcpp`. Use `kobold` or `vllm` for text-to-speech until a `llama-tts` integration exists. llama-server has no `/v1/audio/speech` endpoint.
- **`POST /v1/images/edits`** and **`GET /sdapi/v1/progress`** are KoboldCpp endpoints since `v1.119`.
- KoboldCpp `v1.118.1` removed **Row Split** (`--splitmode row` on CUDA); `v1.117.1` was the last version supporting it, and the RPC protocol changed to match upstream llama.cpp's.

Virtual Jinja kwargs profiles require KoboldCpp v1.114.1 or newer and `chat_template_kwargs` support from the listed llama.cpp baseline. The listed KoboldCpp baseline satisfies that minimum.

## Known, not adopted

- llama.cpp `--host` with several addresses: the router binds each runtime to one loopback address.
- llama.cpp `--log-jsonl`: readiness and load error detection read the text log.
- llama.cpp `LLAMA_ARG_TEMPERATURE` and the other sampling environment variables: the router passes sampling settings on the command line.
- llama.cpp `--spec-synth-len` and `--spec-synth-rates`: benchmarking only.
- KoboldCpp `--analyze` for safetensors files, the reworked MusicUI, the Stable UI LoRA selector and the `KCPP_LORA` container variable: served or used by KoboldCpp itself.
- vLLM features are deferred to a later router release: admission control (`--max-num-queued-reqs`, `--max-num-queued-tokens`), per-request speculative decoding metrics, `reasoning_tokens` usage reporting, `--engram-config`, `--grpc`, and the new model families (DeepSeek-V4.1-Flash, GLM-5.3-Flash, K2-Horizon, Qwen3.8-Flash-Next). Flags without a dedicated `.kcpps` field can still be passed through `vllm.serve_args`.
