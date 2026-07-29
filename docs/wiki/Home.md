# tensors-router Wiki

`tensors-router` presents local AI backends through one base URL and API surface. A client selects a `.kcpps` model configuration, and the router manages the required backend process locally or on a cluster node.

## Start here

- [Getting Started](Getting-Started)
- [Run Topologies](Run-Topologies)
- [Configuration](Configuration)
- [Backends](Backends)

## Operate

- [WebUI](WebUI)
- [Backend WebUI Interfaces](Backend-WebUI-Interfaces)
- [Downloader](Downloader)
- [Cluster Routing](Cluster-Routing)
- [Security and Reverse Proxy](Security-and-Reverse-Proxy)
- [Deployment](Deployment)
- [Testing and Troubleshooting](Testing-and-Troubleshooting)

## Reference

- [Model Configs and Routing](Model-Configs-and-Routing)
- [KCPPS Sharing](KCPPS-Sharing)
- [API Reference](API-Reference)
- [Backend Release Baselines](Backend-Release-Baselines)
- [Cook Backend Options](Cook-Backend-Options)

The router and management WebUI are separate processes. The router can run alone. A master can also run without local models and forward all inference to cluster nodes.
