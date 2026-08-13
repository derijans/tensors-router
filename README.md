# tensors-router

`tensors-router` presents local AI software through one base URL and API surface, similar to the operating model of a cloud AI provider. Clients select a `.kcpps` model configuration, and the router starts, loads, switches, drains, and unloads the required text, image, embedding, voice, or music backend on the local machine or a cluster node. Users do not have to manage each backend process or switch model software by hand.

<table>
  <tr>
    <td><a href="docs/images/webui-live/router.png"><img src="docs/images/webui-live/router.png" alt="Router tab" width="320"></a></td>
    <td><a href="docs/images/webui-live/nodes.png"><img src="docs/images/webui-live/nodes.png" alt="Nodes tab" width="320"></a></td>
    <td><a href="docs/images/webui-live/webuis.png"><img src="docs/images/webui-live/webuis.png" alt="WebUIs tab" width="320"></a></td>
  </tr>
  <tr>
    <td><a href="docs/images/webui-live/download.png"><img src="docs/images/webui-live/download.png" alt="Download tab" width="320"></a></td>
    <td><a href="docs/images/webui-live/models.png"><img src="docs/images/webui-live/models.png" alt="Models tab" width="320"></a></td>
    <td><a href="docs/images/webui-live/benchmarks.png"><img src="docs/images/webui-live/benchmarks.png" alt="Benchmarks tab" width="320"></a></td>
  </tr>
  <tr>
    <td><a href="docs/images/webui-live/analytics.png"><img src="docs/images/webui-live/analytics.png" alt="Analytics tab" width="320"></a></td>
    <td><a href="docs/images/webui-live/cook.png"><img src="docs/images/webui-live/cook.png" alt="Cook tab" width="320"></a></td>
    <td><a href="docs/images/webui-live/recipes.png"><img src="docs/images/webui-live/recipes.png" alt="Recipes tab" width="320"></a></td>
  </tr>
</table>

## Run topologies

- Headless standalone: run the router without the management WebUI.
- WebUI-managed standalone: run the WebUI and let it start and stop the router process.
- Routing-only master: run a master with no local models and place all inference work on cluster nodes.

See [Run topologies](https://github.com/derijans/tensors-router/wiki/Run-Topologies) for configuration and commands.

## Backends

- [KoboldCpp](https://github.com/LostRuins/koboldcpp) provides a single managed process for supported text, image, embedding, voice, and music routes.
- [llama.cpp](https://github.com/ggml-org/llama.cpp) with [stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp) provides separate text and image processes that load independently.
- [vLLM](https://vllm.ai/) uses a separate resident companion with lazy generation, pooling, and speech runtimes. Runtime installation is explicit, isolated, signed-manifest-driven, and available only in supported Linux and Apple Silicon releases.

See [Backends](https://github.com/derijans/tensors-router/wiki/Backends) for supported routes, configuration mapping, and process behavior.

## Documentation

- [Wiki](https://github.com/derijans/tensors-router/wiki)
- [Getting started](https://github.com/derijans/tensors-router/wiki/Getting-Started)
- [Configuration](https://github.com/derijans/tensors-router/wiki/Configuration)
- [API reference](https://github.com/derijans/tensors-router/wiki/API-Reference)
- [Deployment](https://github.com/derijans/tensors-router/wiki/Deployment)
- [Testing and troubleshooting](https://github.com/derijans/tensors-router/wiki/Testing-and-Troubleshooting)
- [Releases](https://github.com/derijans/tensors-router/releases)
- [Router configuration example](config.example.yaml)
- [WebUI configuration example](webui.example.yaml)
- [Downloader configuration example](downloader.example.yaml)
- [License](LICENSE)

## Network scope

This router is intended for intranet use. Do not expose it directly to the public internet. Bind it to localhost or a private interface, restrict `server.allowed_cidrs`, and use a reverse proxy or VPN when remote access is required.

## Credits

Thanks to the maintainers and contributors of the backend projects used by `tensors-router`:

- [KoboldCpp](https://github.com/LostRuins/koboldcpp) provides the combined text, image, embedding, voice, and music backend.
- [llama.cpp](https://github.com/ggml-org/llama.cpp) provides the split text, embedding, multimodal, and supported audio backend.
- [stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp) provides the split image and video backend.
- [vLLM](https://vllm.ai/) provides the generation, pooling, and speech serving engine used by the optional companion backend.
