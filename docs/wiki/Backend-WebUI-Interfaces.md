# Backend WebUI Interfaces

The WebUIs tab exposes browser interfaces supplied by KoboldCpp, llama.cpp, and stable-diffusion.cpp. It does not connect the browser directly to a backend listener. Browser traffic passes through `tensor-router-webui`, then through the router, and finally to the active local or clustered backend.

This provides two ways to use the interfaces:

- Test a backend or `.kcpps` configuration without preparing a separate API client.
- Use a backend's own WebUI as the main interactive frontend while keeping model selection, loading, unloading, authentication, and cluster routing under `tensors-router`.

Normal API clients can continue to use the router base URL at the same time.

## Available interfaces

An interface appears only when the model catalog contains at least one compatible `.kcpps` configuration.

| Backend mode | Interface | Required model lane | Router path |
| --- | --- | --- | --- |
| `kobold` | KoboldCpp Lite | Text-side model | `/router/webuis/kobold-lite/` |
| `kobold` | KoboldCpp llama UI | Text-side model | `/router/webuis/kobold-lcpp/` |
| `kobold` | KoboldCpp StableUI | Image model | `/router/webuis/kobold-sd/` |
| `kobold` | KoboldCpp MusicUI | Music model | `/router/webuis/kobold-music/` |
| `llama_sdcpp` | llama-server UI | Text-side model | `/router/webuis/llama/` |
| `llama_sdcpp` | sd-server UI | Image model | `/router/webuis/sdcpp/` |

Text-side compatibility includes text, embedding, and multimodal configurations. KoboldCpp voice configurations are also included in its text-side catalog.

The backend binary must contain the named interface. The router catalog cannot add an interface that is absent from the selected backend build.

## Configure the listeners

Backend interfaces require `tensor-router-webui`. Configure its management and backend interface listeners in `webui.yaml`:

```yaml
server:
  bind: "127.0.0.1:8443"
  backend_ui_bind: "127.0.0.1:8444"
  backend_ui_public_url: ""
```

`server.bind` serves the management WebUI. `server.backend_ui_bind` serves proxied backend interfaces on a separate HTTPS origin. The two listeners must use different addresses.

For local access, the default loopback listeners are sufficient. Open the management interface at `https://127.0.0.1:8443`; backend interfaces open on `https://127.0.0.1:8444`.

When a reverse proxy, container port mapping, or NAT changes the browser-visible backend interface address, set `server.backend_ui_public_url` to its external HTTPS origin:

```yaml
server:
  backend_ui_public_url: "https://router.example.test:8444"
```

The value must contain only an HTTPS origin. Credentials, paths, queries, and fragments are rejected. Ensure the backend interface port is reachable from the browser and covered by the configured certificate or `server.cert_hosts`.

## Enable and open an interface

1. Start the router and `tensor-router-webui`.
2. Open the management WebUI and select the WebUIs tab.
3. Find the interface for the required backend mode and model lane. In a cluster, each compatible model row identifies its node.
4. Turn on its Enable toggle.
5. If a compatible model is already active, select Open.
6. If no compatible model is active, select Resolve or Models, then select Load beside a model.
7. After the backend reports ready, the interface opens in a new browser tab. If it does not open automatically, select Open.

Loading from this tab performs a normal router model load. It starts the required lazy backend process, resolves portable assets when needed, loads the selected `.kcpps` configuration, and waits for the correct text or image readiness endpoint.

Enablement only permits the proxy route. It does not start a backend or load a model. The enabled state is stored in router memory and resets when the router restarts. Enable only the interfaces needed for the current session.

## Use for backend tests

The native interface is useful for checking whether a backend build and model configuration work together. Select the exact model in the WebUIs tab, load it, then exercise the backend's own controls.

Requests made by the native interface are restricted to the API paths assigned to that interface and are forwarded to the same loaded backend runtime used by router API clients. This can verify:

- backend startup and readiness
- text, image, embedding, voice, or music assets used by the interface
- request streaming and response rendering
- backend-specific options exposed by its own UI
- local and clustered backend routing

An interface test does not replace direct API testing. Use [API Reference](API-Reference) and [Testing and Troubleshooting](Testing-and-Troubleshooting) when the request body, response format, authentication, or client compatibility must be checked directly.

## Use as the main interactive frontend

The backend interface may remain enabled and be used as the normal browser client. Its API calls still pass through the router, so the browser does not need the backend's loopback URL and the backend port does not need to be exposed.

Keep these constraints in mind:

- The interface works with the currently active compatible runtime.
- Selecting Load in the WebUIs tab is the supported way to switch its model.
- A model switch follows the same drain and unload policy as an API-triggered switch.
- Other clients can use the router API concurrently when their requests are compatible with the active runtime.
- Enabling an interface does not change which inference or administration credentials protect the normal router APIs.

## Cluster behavior

A master lists compatible backend interfaces from local and registered slave nodes. Loading a remote entry sends the model load through the cluster-authenticated node API. The browser remains connected to the master's backend interface origin while requests are forwarded to the active slave backend.

The slave backend listener remains private. Redirects and backend API paths are rewritten so navigation stays under the selected `/router/webuis/...` route.

## Access controls

In secure mode, opening an interface from the management WebUI creates a short-lived, single-use ticket. The backend interface listener exchanges it for a secure, HTTP-only session limited to that interface type. The session expires after 15 minutes.

Management cookies, router bearer tokens, and caller authorization headers are not forwarded to backend processes. The proxy also removes hop-by-hop headers and exposes only the backend API path groups required by each interface.

In `trusted_lan` mode, the ticket exchange is skipped. Listener separation, backend loopback isolation, cluster authentication, path filtering, and transport limits remain active.

See [Security and Reverse Proxy](Security-and-Reverse-Proxy) before making either WebUI listener reachable outside the local machine.
