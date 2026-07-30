# Configuration

The repository contains three canonical examples:

- [`config.example.yaml`](https://github.com/derijans/tensors-router/blob/main/config.example.yaml) for `tensors-router`
- [`webui.example.yaml`](https://github.com/derijans/tensors-router/blob/main/webui.example.yaml) for `tensor-router-webui`
- [`downloader.example.yaml`](https://github.com/derijans/tensors-router/blob/main/downloader.example.yaml) for `tensor-router-downloader`

The parsers reject unknown sections and fields. Relative paths are resolved from the directory containing the corresponding YAML file unless a field says otherwise. Durations use Go duration syntax such as `30s`, `3m`, or `168h`.

## Router configuration

### Security and server

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `security.profile` | String enum: `secure`, `trusted_lan` | `secure` | Selects authentication and network defaults. |
| `server.bind` | String `host:port` | `127.0.0.1:8080` | Router HTTP listener. |
| `server.allowed_cidrs` | List of CIDR strings | Loopback and RFC 1918 ranges | Limits accepted client addresses before authentication. |

### Authentication

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `auth.inference_keys` | List of nonempty strings | `[]` | Bearer keys accepted by model inference APIs. Duplicates are rejected. |
| `auth.admin_keys` | List of nonempty strings | `[]` | Bearer keys accepted by management APIs. Duplicates are rejected. |
| `auth.bearer_keys` | List of nonempty strings | `[]` | Deprecated inference-key alias retained for existing configurations. |

### Models and assets

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `models.config_dir` | Path string | `./kcpps` | Directory containing `.kcpps` files. It must exist when the router starts. |
| `models.startup_model` | String model ID | Empty | Model to load at startup. Empty leaves no requested startup model. |
| `models.shared_dir` | Path string | Empty | Shared asset destination. Empty uses `cluster.store_dir/model-assets`. |
| `models.hash_workers` | Integer, at least `1` | `1` | Maximum concurrent workers used to hash model files. |
| `models.concurrent_asset_transfers` | Integer, at least `1` | `2` | Maximum concurrent model asset transfers. |
| `models.file_roots` | List of nonempty path strings | `./models` in the example | Roots exposed to inventory, hashing, and portable asset resolution. |

### Backend selection

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `backend.mode` | String enum: `kobold`, `llama_sdcpp` | `kobold` | Default backend family. A `.kcpps` `backend_mode` can override it per model. |

## MCP artifacts

| Key | Type | Default | Meaning |
|---|---|---:|---|
| `mcp.enabled` | Boolean | `false` | Enables materialization of MCP definitions stored in model configurations. |
| `mcp.directory` | Path string | `./mcp` | Artifact directory, resolved relative to this router configuration file. |

MCP server definitions remain embedded in the `.kcpps` source. When active, the router writes a private, generated `servers.json` at `<mcp.directory>/<config-stem>/servers.json`. Disabling the global setting or a model's `mcp_enabled` removes that generated directory while retaining the embedded definition. Treat any configuration that contains `mcp_servers` as secret-bearing, including disabled definitions.

### KoboldCpp process

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `kobold.backend_url` | Loopback HTTP URL string | `http://127.0.0.1:5001` | Managed KoboldCpp endpoint. |
| `kobold.binary_path` | Path string | `./bin/kobold/koboldcpp` | KoboldCpp executable. |
| `kobold.data_dir` | Path string | `./data` | Working and data directory for the process. |
| `kobold.multiuser` | Integer, at least `1` | `1` | Value supplied to KoboldCpp multiuser handling. |
| `kobold.quiet` | Boolean | `true` | Enables the managed quiet option. |
| `kobold.skip_launcher` | Boolean | `true` | Skips the native KoboldCpp launcher UI. |
| `kobold.no_model` | Boolean | `true` | Starts without a model so the router controls model loading. |
| `kobold.hide_window` | Boolean | `true` | Hides the child process window on supported platforms. |
| `kobold.extra_args` | List of strings | `[]` | Additional backend arguments. Managed host and port options cannot be overridden. |

### llama.cpp process

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `llama.backend_url` | Loopback HTTP URL string | `http://127.0.0.1:5002` | Managed `llama-server` endpoint. |
| `llama.binary_path` | Path string | `./bin/llama/llama-server` | `llama-server` executable. |
| `llama.data_dir` | Path string | `./data/llama` | Working and data directory for the text process. |
| `llama.hide_window` | Boolean | `true` | Hides the child process window on supported platforms. |
| `llama.extra_args` | List of strings | `[]` | Additional arguments. Managed host and port options cannot be overridden. |

### stable-diffusion.cpp process

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `sdcpp.backend_url` | Loopback HTTP URL string | `http://127.0.0.1:7860` | Managed `sd-server` endpoint. |
| `sdcpp.binary_path` | Path string | `./bin/stable-diffusion/build/bin/sd-server` | `sd-server` executable. |
| `sdcpp.data_dir` | Path string | `./data/sdcpp` | Working and data directory for the image process. |
| `sdcpp.hide_window` | Boolean | `true` | Hides the child process window on supported platforms. |
| `sdcpp.extra_args` | List of strings | `[]` | Additional arguments. Managed listen address and port options cannot be overridden. |

### Logging

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `logging.mode` | String enum: `normal`, `startup_only`, `quiet` | `normal` | Controls router log output. |
| `logging.backend_logs_to_disk` | Boolean | `false` | Writes managed backend output to files instead of retaining startup output only. |
| `logging.enabled` | Boolean | `true` | Deprecated compatibility field mapped to a logging mode. |

### Backend updates

When updates are enabled, each selected backend needs either a direct binary URL or a repository URL. All configured URLs must use HTTPS. A SHA-256 value is optional, but when present it must contain exactly 64 hexadecimal characters.

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `updates.enabled` | Boolean | `false` | Enables periodic backend binary checks and replacement. |
| `updates.check_interval` | Positive duration string | `168h` | Time between update checks. |
| `updates.include_prereleases` | Boolean | `false` | Allows prerelease GitHub releases when repository resolution is used. |
| `updates.binary_url` | Empty or HTTPS URL string | KoboldCpp direct URL in the example | Direct KoboldCpp binary source. |
| `updates.binary_sha256` | Empty or 64-character SHA-256 string | Empty | Expected digest for a direct KoboldCpp download. |
| `updates.binary_repository_url` | Empty or HTTPS repository URL string | Empty | KoboldCpp release repository source. |
| `updates.binary_asset_glob` | String glob | Empty | Selects a KoboldCpp release asset. |
| `updates.llama_binary_url` | Empty or HTTPS URL string | Empty | Direct llama.cpp binary source. |
| `updates.llama_binary_sha256` | Empty or 64-character SHA-256 string | Empty | Expected digest for a direct llama.cpp download. |
| `updates.llama_repository_url` | Empty or HTTPS repository URL string | `https://github.com/ggml-org/llama.cpp` | llama.cpp release repository source. |
| `updates.llama_asset_glob` | String glob | Empty | Selects a llama.cpp release asset. |
| `updates.sdcpp_binary_url` | Empty or HTTPS URL string | Empty | Direct stable-diffusion.cpp binary source. |
| `updates.sdcpp_binary_sha256` | Empty or 64-character SHA-256 string | Empty | Expected digest for a direct stable-diffusion.cpp download. |
| `updates.sdcpp_repository_url` | Empty or HTTPS repository URL string | `https://github.com/leejet/stable-diffusion.cpp` | stable-diffusion.cpp release repository source. |
| `updates.sdcpp_asset_glob` | String glob | Empty | Selects a stable-diffusion.cpp release asset. |

### Downloader companion

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `downloader.enabled` | Boolean | `true` | Exposes downloader management when the companion is available. |
| `downloader.binary_location` | Path string | Empty | Downloader executable. Empty searches beside the router executable. |

### Cluster

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `cluster.role` | String enum: `standalone`, `master`, `slave` | `standalone` | Selects the node's routing role. |
| `cluster.node_id` | Nonempty string | `local` | Stable node identity. It must be unique in a cluster. |
| `cluster.public_url` | Empty or absolute URL string | Empty | Reachable URL advertised by a slave. Required for a slave. |
| `cluster.master_url` | Empty or absolute URL string | Empty | Master URL used by a slave. Required for a slave. |
| `cluster.slave_urls` | List of absolute URL strings | `[]` | Slave URLs permitted and polled by a master. |
| `cluster.token` | String | Empty | Shared cluster credential. Required for master and slave roles and rejected when it is a placeholder. |
| `cluster.store_dir` | Path string | `./router-store` | Stores registry, asset index, recipes, benchmarks, and default analytics data. |
| `cluster.sync_interval` | Positive duration string | `60s` | Interval between slave registry synchronization attempts. |
| `cluster.health_interval` | Positive duration string | `15s` | Interval between cluster health checks. |

### Analytics

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `analytics.enabled` | Boolean | `false` | Enables persisted request and runtime analytics. |
| `analytics.vram_enabled` | Boolean | `true` | Enables VRAM sampling when analytics is active. |
| `analytics.flush_interval` | Positive duration string | `3m` | Interval between analytics aggregation flushes. |
| `analytics.database_path` | Path string | Empty | SQLite path. Empty uses `cluster.store_dir/analytics.sqlite`. |
| `analytics.raw_retention` | Positive duration string | `720h` | Retention period for raw analytics samples. |
| `analytics.vram_sample_interval` | Positive duration string | `1s` | Interval between VRAM samples. |

### Request and memory limits

All size fields are positive integers. Replay and selector scan limits cannot exceed the maximum stream request size. The memory budget must cover twice the replay buffer plus the router's transformation working set.

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `limits.max_control_body_mb` | Positive integer, MiB | `8` | Maximum buffered control request body. |
| `limits.replay_buffer_mb` | Positive integer, MiB | `64` | Maximum body retained for replay while selecting or retrying a backend. |
| `limits.memory_budget_mb` | Positive integer, MiB | `2048` | Total router budget for bounded in-memory request processing. |
| `limits.max_stream_request_gb` | Positive integer, GiB | `32` | Maximum streamed request body. |
| `limits.max_stream_response_gb` | Positive integer, GiB | `32` | Maximum streamed backend response. |
| `limits.selector_scan_mb` | Positive integer, MiB | `64` | Maximum prefix scanned to locate a model selector. |
| `limits.drain_timeout` | Positive duration string | `15m` | Maximum graceful drain time during unload, restart, or shutdown. |

## WebUI configuration

### Security and listeners

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `security.profile` | String enum: `secure`, `trusted_lan` | `secure` | Selects WebUI authentication behavior. |
| `server.bind` | String `host:port` | `127.0.0.1:8443` | HTTPS management listener. |
| `server.backend_ui_bind` | String `host:port` | `127.0.0.1:8444` | Separate listener for proxied backend interfaces. It must differ from `server.bind`. |
| `server.backend_ui_public_url` | Empty or HTTPS origin string | Empty | Browser-visible backend UI origin for NAT or reverse proxy use. Credentials, path, query, and fragment are rejected. |
| `server.state_dir` | Path string | `./webui-state` | Stores generated certificates and WebUI state. |
| `server.cert_file` | Path string | Empty | TLS certificate file. Empty uses a generated certificate. |
| `server.key_file` | Path string | Empty | TLS private-key file. It must be configured with `cert_file`. |
| `server.cert_hosts` | List of host or IP strings | `[]` | Extra names and addresses added to a generated certificate. |
| `server.admin_token` | String | Empty | WebUI login credential. Required for a secure non-loopback bind and rejected when it is a placeholder. |

See [Backend WebUI Interfaces](Backend-WebUI-Interfaces) for listener setup, external origins, enablement, and browser access.

### Router process

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `router.url` | Empty or HTTP/HTTPS URL string | Empty | External router URL. Empty enables managed-process mode. |
| `router.token` | String | Empty | Bearer token sent to the router management API. |
| `router.binary_path` | Path string | `./tensors-router` | Router executable used in managed mode. |
| `router.config_path` | Path string | `./config.yaml` | Router YAML passed to a managed process. |
| `router.start_when_missing` | Boolean | `true` | Starts the managed router when no process is available. |
| `router.shutdown_with_webui` | Boolean | `true` | Stops the managed router during WebUI shutdown. |
| `router.args` | List of strings | `[]` | Extra router arguments. They cannot override the managed security profile. |

### Logging

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `logging.mode` | String enum: `normal`, `startup_only`, `quiet` | `normal` | Controls WebUI process logs. |
| `logging.enabled` | Boolean | `true` | Deprecated compatibility field mapped to a logging mode. |

## Downloader configuration

### Storage and Hugging Face

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `storage.root` | Required path string | `./models` | Destination root for downloaded repositories and files. |
| `storage.state_dir` | Required path string | `./downloader-state` | Stores downloader state. |
| `storage.database_path` | Required path string inside `state_dir` | `./downloader-state/downloads.sqlite` | SQLite job and artifact database. |
| `storage.free_space_reserve_gb` | Integer, at least `0`, GiB | `5` | Space that download planning must leave unused. |
| `huggingface.token` | String | Empty | Optional token for private or gated Hugging Face repositories. |

### Jobs and scanning

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `downloads.concurrent_jobs` | Integer, at least `1` | `2` | Maximum download jobs executed together. |
| `downloads.concurrent_files` | Integer, at least `1` | `4` | Maximum files downloaded together within the job manager. |
| `downloads.retry_limit` | Integer, at least `0` | `5` | Maximum retry count after a failed file transfer. |
| `downloads.timeout` | Positive duration string | `30s` | Timeout used for Hub requests and download operations. |
| `scanning.hash_workers` | Integer, at least `1` | `1` | Maximum workers used while hashing local artifacts. |
| `scanning.write_hash_sidecars` | Boolean | `true` | Writes hash sidecar files beside indexed artifacts. |

### Hardware estimates and logging

| Field | Type or options | Example or default | Description |
| --- | --- | --- | --- |
| `hardware.default_context` | Integer, at least `1` | `8192` | Context length used by download planning estimates when model metadata does not supply one. |
| `hardware.vram_reserve_mb` | Integer, at least `0`, MiB | `1024` | VRAM reserved from model-fit estimates. |
| `hardware.safety_margin_percent` | Integer from `0` through `99` | `15` | Additional margin applied to hardware-fit estimates. |
| `logging.mode` | String enum: `normal`, `startup_only`, `off` | `normal` | Controls downloader logs. The log file path is derived from the downloader configuration location. |

## Precedence

The router and WebUI security profiles use this order:

1. Command-line option
2. `TENSORS_ROUTER_SECURITY_PROFILE`
3. Configuration file
4. Secure default

Other command-line and environment overrides are resolved by the corresponding executable before validation.
