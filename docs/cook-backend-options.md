# Cook Backend Options

Status: review draft before further catalog expansion.

Rules:
- Documented backend keys are verified and must not produce `unverified_option`.
- Observed unknown keys remain editable under `Other`.
- Null, empty string, and missing values are ignored for comparison coloring.
- Every field supports custom input even when dropdown values exist.

## Sources
- KoboldCPP source: https://github.com/LostRuins/koboldcpp/blob/concedo/koboldcpp.py
- KoboldCPP wiki: https://github.com/LostRuins/koboldcpp/wiki
- llama.cpp server README: https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md
- stable-diffusion.cpp server README: https://github.com/leejet/stable-diffusion.cpp/blob/master/examples/server/README.md
- stable-diffusion.cpp CLI README: https://github.com/leejet/stable-diffusion.cpp/blob/master/examples/cli/README.md

## Common / Runtime

| Key | Type | Values | Notes |
| --- | --- | --- | --- |
| `baseconfig` | path | node `.kcpps` files + custom | Verified KoboldCPP admin/base config option. |
| `config` | path | node `.kcpps` files + custom | Load settings from config. |
| `host` | string | node backend host + custom | Router-owned when managed. |
| `port` | number | node backend port + custom | Router-owned when managed. |
| `quiet` | bool | `true`, `false` | Prepopulate `true`. |
| `launch`, `showgui`, `skiplauncher` | bool | `true`, `false` | UI/launcher behavior. |
| `admin`, `adminpassword`, `admindir`, `adminunloadtimeout` | mixed | observed + custom | Admin reload support. |
| `routermode`, `autoswapmode`, `reqtimeout` | mixed | observed + custom | Kobold router/admin behavior. |
| `password`, `ssl`, `nocertify`, `remotetunnel` | mixed | observed + custom | Security/exposure fields. |
| `multiuser`, `multiplayer`, `websearch`, `maxrequestsize` | mixed | observed + custom | Server behavior. |
| `onready`, `preloadstory`, `savedatafile`, `mcpfile`, `downloaddir` | path/string | node files + custom | Runtime side files. |
| `device`, `rpcmode`, `rpcport`, `rpchost`, `rpcdevice`, `rpctargets` | mixed | `disabled`, `connect`, `host` where applicable | Device/RPC fields. |

## LLM

| Key | Type | Values | Notes |
| --- | --- | --- | --- |
| `model`, `model_param` | model path | node LLM files + custom | Model dropdown. |
| `nomodel` | bool | `true`, `false` | Required for image-only configs. |
| `threads`, `blasthreads` | number | hardware-derived + custom | CPU thread controls. |
| `contextsize` | number | `4096`, `8192`, `16384`, `32768`, `65536`, custom | Kobold range 256-262144. |
| `gpulayers` | number/string | `-1`, `0`, `auto`, `all`, custom | Backend-specific. |
| `batchsize` | number | `-1`, `16`, `32`, `64`, `128`, `256`, `512`, `1024`, `2048`, `4096` | Kobold and llama.cpp verified. |
| `splitmode` | enum | `none`, `layer`, `row`, `tensor` | llama.cpp verified. |
| `tensor_split` | list/string | observed + custom | Multi-GPU ratios. |
| `maingpu` | number/string | GPU indexes, `main`, `CPU`, custom | GPU picker. |
| `usecuda`, `usevulkan`, `usecpu` | mixed | detected backend + custom | GPU backend choice. |
| `usemmap`, `usemlock`, `lowvram`, `nommq`, `autofit` | bool | `true`, `false` | Memory behavior. |
| `quantkv` | enum | `f16`, `bf16`, `q8_0`, `q5_1`, `q4_0`, `0`, `1`, `2`, `3` | Kobold verified. |
| `cache_type_k`, `cache_type_v` | enum | `f32`, `f16`, `bf16`, `q8_0`, `q4_0`, `q4_1`, `iq4_nl`, `q5_0`, `q5_1` | llama.cpp server. |
| `flashattention`, `noflashattention` | bool | `true`, `false` | Compatibility/deprecated handling. |
| `ropeconfig`, `overridenativecontext`, `overridekv`, `overridetensors` | mixed | observed + custom | Advanced model metadata. |
| `lora`, `loramult`, `draftmodel`, `draftamount`, `draftgpulayers`, `draftgpusplit` | mixed | node files + custom | LLM LoRA/speculative decode. |
| `jinja`, `jinja_tools`, `jinja_kwargs`, `jinjatemplate`, `jinjathink` | mixed | `default`, `true`, `false` where applicable | Template fields. |

## Multimodal / Embeddings

| Key | Type | Values | Notes |
| --- | --- | --- | --- |
| `mmproj` | model path | node multimodal files + custom | Projector file. |
| `mmprojcpu` | bool | `true`, `false` | Projector CPU/offload. |
| `visionmaxres` | number | `512`, `768`, `1024`, `1536`, `2048`, custom | Kobold range. |
| `visionmintokens`, `visionmaxtokens` | number | `-1`, observed + custom | Vision token controls. |
| `embeddingsmodel` | model path | node embedding files + custom | Embedding model dropdown. |
| `embeddingsmaxctx` | number | `512`, `1024`, `2048`, `4096`, `8192`, custom | Default 4096 in Kobold. |
| `embeddingsgpu` | bool | `true`, `false` | GPU offload. |
| `pooling` | enum | `none`, `mean`, `cls`, `last`, `rank` | llama.cpp embedding/rerank. |

## Image

| Key | Type | Values | Notes |
| --- | --- | --- | --- |
| `sdmodel` | model path | node image files + custom | Image model dropdown. |
| `sdvae`, `sdvaeauto`, `sdaudiovae` | mixed | node VAE files, `true`, `false`, custom | VAE fields. |
| `sdt5xxl`, `sdclip1`, `sdclip2`, `sdclipl`, `sdclipg`, `sdphotomaker`, `sdupscaler` | path | node files by role + custom | Encoder/upscaler fields. |
| `sdlora`, `sdloramult` | mixed | node LoRA files + custom | Multiple values allowed. |
| `sdthreads` | number | hardware-derived + custom | Image CPU threads. |
| `sdquant` | enum | `0`, `1`, `2` | Kobold: off/q8/q4. |
| `sdtiledvae` | number | `0`, `512`, `768`, `1024`, custom | 0 disables. |
| `sdmaingpu`, `sdvaedevice`, `sdclipdevice` | string/number | GPU indexes, `main`, `CPU`, custom | Device dropdown. |
| `sdflashattention`, `sdoffloadcpu` | bool | `true`, `false` | Image memory/perf. |
| `sdconvdirect` | enum | `off`, `full`, `vaeonly` | Kobold verified. |
| `sampling_method` | enum | `euler`, `euler_a`, `heun`, `dpm2`, `dpm++2s_a`, `dpm++2m`, `dpm++2mv2`, `ipndm`, `ipndm_v`, `lcm`, `ddim_trailing`, `tcd`, `res_multistep`, `res_2s`, `er_sde`, `euler_cfg_pp`, `euler_a_cfg_pp` | stable-diffusion.cpp. |
| `scheduler` | enum | `discrete`, `karras`, `exponential`, `ays`, `gits`, `smoothstep`, `sgm_uniform`, `simple`, `kl_optimal`, `lcm`, `bong_tangent`, `ltx2` | stable-diffusion.cpp. |
| `type` | enum | `f32`, `f16`, `q4_0`, `q4_1`, `q5_0`, `q5_1`, `q8_0`, `q2_K`, `q3_K`, `q4_K` | stable-diffusion.cpp. |
| `rng`, `sampler_rng` | enum | `std_default`, `cuda`, `cpu` | stable-diffusion.cpp. |
| `prediction`, `lora_apply_mode` | enum | `eps`, `v`, `edm_v`, `sd3_flow`, `flux_flow`, `flux2_flow`; `auto`, `immediately`, `at_runtime` | stable-diffusion.cpp. |

## Voice / Audio / Music

| Key | Type | Values | Notes |
| --- | --- | --- | --- |
| `whispermodel` | model path | node `.bin` files + custom | Speech-to-text. |
| `ttsmodel`, `ttswavtokenizer`, `ttsdir` | path | node GGUF/files/directories + custom | TTS. |
| `ttsgpu` | bool | `true`, `false` | TTS GPU. |
| `ttsmaxlen`, `ttsthreads` | number | observed + custom | TTS runtime. |
| `musicllm`, `musicembeddings`, `musicdiffusion`, `musicvae` | path | node model files + custom | Music generation. |
| `musiclowvram` | bool | `true`, `false` | Music memory behavior. |
