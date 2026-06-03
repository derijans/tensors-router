# KoboldCpp vs llama.cpp + stable-diffusion.cpp

## Backend Mode

`backend.mode: "kobold"` keeps the original single KoboldCpp backend. The router starts KoboldCpp in admin mode, reloads `.kcpps` files through the KoboldCpp admin endpoint, and sends both text and image requests to the same process.

`backend.mode: "llama_sdcpp"` splits the work into two native lanes. LLM, embedding, and multimodal requests go to `llama-server`; image requests go to `sd-server`.

## Process Lifecycle

Kobold mode starts one no-model KoboldCpp process at router startup. Model switches reload that process.

Split mode starts native workers lazily when a request or explicit load selects a `.kcpps` file. Text and image lanes switch independently. A lane switch waits for in-flight requests in that lane to finish, then stops the old process and starts the new one.

## Backend Downloads

Kobold mode downloads `kobold.binary_path` from `updates.binary_url` and verifies `updates.binary_sha256`.

Split mode downloads `llama.binary_path` from `updates.llama_binary_url` and `sdcpp.binary_path` from `updates.sdcpp_binary_url`, then verifies `updates.llama_binary_sha256` and `updates.sdcpp_binary_sha256`. Download URLs must use HTTPS. Each binary has separate update metadata in its own data directory, so a fresh llama.cpp download does not suppress a needed stable-diffusion.cpp download.

## Endpoint Coverage

Kobold mode keeps the existing proxied endpoints, including `/v1/...`, `/sdapi/v1/...`, and the router-recognized ComfyUI-style image paths.

Split mode routes `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`, and other OpenAI text paths to `llama-server`. It routes `/v1/images/...`, `/sdapi/v1/...`, and `/sdcpp/v1/...` to `sd-server`. ComfyUI-style paths are still classified as image requests by the router, but `sd-server` does not implement those endpoints.

## `.kcpps` Mapping

The router still reads `.kcpps` files from `models.config_dir`.

For llama.cpp, it maps text fields such as `model_param`, `model`, `embeddingsmodel`, `mmproj`, `contextsize`, `threads`, `batchsize`, `gpulayers`, `splitmode`, `tensor_split`, `maingpu`, `usemmap`, `usemlock`, `quantkv`, and vision token limits to `llama-server` arguments where supported.

For stable-diffusion.cpp, it maps image fields such as `sdmodel`, `sdvae`, `sdt5xxl`, `sdclip1`, `sdclipl`, `sdclip2`, `sdclipg`, `sdupscaler`, `sdthreads`, `sdflashattention`, `sdoffloadcpu`, `sdvaecpu`, and `sdtiledvae` to `sd-server` arguments where supported.

Per-backend `extra_args` are appended last so explicit config values can override inferred arguments.

## Cluster Behavior

Cluster model records now include `backend_mode`. Masters use text lane health for LLM, embedding, and multimodal routes, and image lane health for image routes.

In split mode, combined LLM+image `.kcpps` configs can expose their image model without requiring the text lane to have that config active.
