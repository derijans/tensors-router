# Backend Release Baselines

Audit completed: 2026-08-26.

| Backend | Source | Verified marker |
| --- | --- | --- |
| KoboldCpp | https://github.com/LostRuins/koboldcpp/releases | `v1.119` |
| stable-diffusion.cpp | https://github.com/leejet/stable-diffusion.cpp/releases | `master-830-50d6405` |
| llama.cpp | https://github.com/ggml-org/llama.cpp/releases | `b10636` |
| whisper.cpp | https://github.com/ggml-org/whisper.cpp/releases/tag/v1.9.3 | `v1.9.3` |
| vLLM | https://github.com/vllm-project/vllm/releases | `v0.28.0` |

The listed markers are compatibility baselines for this router release, not a promise that they remain upstream's newest tags.

## Per-feature minimums

- **MiniMax-H3 video generation** requires KoboldCpp `v1.119` or stable-diffusion.cpp `master-812-ea7f0c8` or newer. H3 is video-only: both backends reject it for still-image generation. `sdaudiovae` (`--audio-vae`) must be set for H3's audio track; KoboldCpp only preserves that audio in its MJPG-AVI video output, not its GIF output.
- **`minimax-01` GGUF architecture** (`MiniMaxText01ForCausalLM`, `MiniMaxM1ForCausalLM`) requires llama.cpp `b10437` or newer, or a KoboldCpp release built from it (`v1.119` or newer). `minimax-m2` and `minimax-m3` were already supported at the prior baseline.
- **`--model-vocoder` and `--model-talker` were removed from llama.cpp** on 2026-08-04 (TTS moved to the `llama-tts` CLI via mtmd). The router no longer emits either flag, and `.kcpps` fields that depended on them (`ttsmodel`, `ttswavtokenizer`, `talkermodel`, `code2wavmodel`) are rejected under `backend_mode: llama_sdcpp` — use `kobold` or `vllm` for text-to-speech until a `llama-tts` integration exists. This also means `llama_sdcpp` never had a working `/v1/audio/speech` route: current llama-server has no such endpoint at all.
- **`POST /v1/images/edits`** and **`GET /sdapi/v1/progress`** are new KoboldCpp endpoints as of `v1.119`.
- KoboldCpp `v1.118.1` removed **Row Split** (`--splitmode row` on CUDA); `v1.117.1` was the last version supporting it, and the RPC protocol changed to match upstream llama.cpp's.
- vLLM **removed** the `MiniMaxText01ForCausalLM` and `MiniMaxM1ForCausalLM` architectures in `0.23.0` (kept only `MiniMaxM2ForCausalLM` and the M3 family) — the inverse of llama.cpp, which only just gained them.

Virtual Jinja kwargs profiles require KoboldCpp v1.114.1 or newer and `chat_template_kwargs` support from the listed llama.cpp baseline. The listed KoboldCpp baseline satisfies that minimum.
