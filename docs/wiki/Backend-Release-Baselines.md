# Backend Release Baselines

Audit completed: 2026-07-30.

| Backend | Source | Verified marker |
| --- | --- | --- |
| KoboldCpp | https://github.com/LostRuins/koboldcpp/releases | `v1.117.1` |
| stable-diffusion.cpp | https://github.com/leejet/stable-diffusion.cpp/releases | `master-802-e92e86f` |
| llama.cpp | https://github.com/ggml-org/llama.cpp/releases | `b10189` |

The stable-diffusion.cpp checkpoint-overflow fix at `master-802-e92e86f` was reviewed. The listed markers are the compatibility baselines for this router release, not a promise that they remain upstream's newest tags.

Virtual Jinja kwargs profiles require KoboldCpp v1.114.1 or newer and `chat_template_kwargs` support from the listed llama.cpp baseline. The listed KoboldCpp baseline satisfies that minimum.
