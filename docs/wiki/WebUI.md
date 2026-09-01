# WebUI

`tensor-router-webui` is optional. It serves the management interface and connects to a router process.

## Process layout

When `router.url` is empty, the WebUI can start `tensors-router` from `router.binary_path` with `router.config_path`. It stops that managed process on exit when `router.shutdown_with_webui` is enabled.

When `router.url` is set, the router is external. Launch, restart, and kill controls are disabled because the WebUI does not own that process.

## HTTPS listeners

The management interface uses `server.bind`. Backend-native interfaces use the separate `server.backend_ui_bind` listener.

Do not assign both listeners the same address. Keeping them separate prevents management credentials and backend content from sharing an origin.

When certificate files are absent, the WebUI creates a self-signed certificate in `server.state_dir`. Generated certificates include localhost, loopback addresses, and local interface addresses for wildcard binds. Add browser-facing DNS names or external addresses to `server.cert_hosts` when they cannot be inferred.

Set `server.backend_ui_public_url` when NAT or a reverse proxy changes the browser-visible backend UI origin. It must be an HTTPS origin without credentials, path, query, or fragment.

Secure mode uses short-lived, single-use tickets when the browser crosses from the management origin to a backend UI origin. Router admin cookies and caller credentials are not forwarded to backend processes.

## Authentication

Secure non-loopback WebUI binds require `server.admin_token`.

Trusted-LAN mode skips WebUI login and CSRF authentication. Router cluster authentication, listener isolation, header filtering, CIDR checks, and resource limits remain active.

The WebUI and an external router validate their security profiles independently. A managed WebUI passes its effective profile to the child router.

## Backend WebUI interfaces

The WebUIs tab can expose the native interface supplied by the selected backend. These interfaces can be used for backend tests or as the regular interactive frontend while requests continue through the router.

See [Backend WebUI Interfaces](Backend-WebUI-Interfaces) for supported interfaces, configuration, enablement, model loading, cluster behavior, and access controls.

## Model inventory and cooking

The Models tab requests a file inventory from configured `models.file_roots`. Inventory refreshes can hash configuration and model assets, so they are separate from the lightweight node inventory used by the Nodes tab.

Cooking can create a normal `.kcpps` file on one node or a split recipe on the master when selected components live on different nodes.

Portable export replaces local model paths with verified asset references for sharing. See [KCPPS Sharing](KCPPS-Sharing).

The Routing column on each image model opens its routing group. The dialog lists every image model on the other nodes, unfiltered, because the models worth grouping are usually the ones a name or hash filter would hide. Each candidate is labelled as the same weights or different weights, compared against the anchor by model hash. Selecting a candidate with different weights requires an explicit acknowledgement before the group can be saved, since the router cannot detect that it returns different images. Clearing every candidate deletes the group. See [Cluster Routing](Cluster-Routing) for what a group changes about scheduling.

The Separate column on each kobold or `llama_sdcpp` model opens its separate-runtime settings for that node. Toggling "Run in its own process" moves the config into the [separate-runtime pool](Backends#separate-runtimes). The trigger groups — lanes, backend families, and named sibling configs — choose which loads on the shared runtime evict the pooled runtime; "Do not unload" means no trigger evicts it, though a full pool still unloads the least-recently-used one. The setting is stored per node in the model-state database and never rewrites the `.kcpps`.

## Load captures

The Load Captures tab appears when at least one selected node has `analytics.load_capture_enabled`. It can filter and merge attempt summaries across nodes, inspect sanitized KCPPS and asset identities, and fetch bounded stdout/stderr output incrementally. Reused loads link back to their physical attempt output.

The capture database is node-local. A node that disables capture remains absent from the viewer and does not receive capture storage writes.

See [Cook Backend Options](Cook-Backend-Options) for the option catalog.
