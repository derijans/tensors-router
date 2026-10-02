# Downloader

The downloader is a separate companion executable. The router is the source of truth for its availability and exposes downloader management routes according to `downloader.enabled`.

If `downloader.binary_location` is empty, the router looks for the companion beside its own executable. A configured relative path is resolved from the router configuration directory.

The companion reads `downloader.yaml` next to the router configuration, or the file named by `downloader.config_path`. When that file is missing the downloader still starts with defaults (`models/` and `downloader-state/` beside it) and logs a warning. Give each node on one machine its own directory or `config_path` so their libraries and job databases stay separate.

The router supervises the companion. If it exits, requests report the outage with HTTP 503 while the router restarts it with backoff, and interrupted jobs continue on their own.

The downloader storage root is added to `models.file_roots` automatically, so downloaded files appear in the model inventory and can be cooked into `.kcpps` configurations without extra configuration.

At startup the router always logs downloader status with `enabled`, `present`, and `working` fields. A ready installation reports `downloader status enabled=true present=true working=true`. Disabled or failed initialization also includes an actionable `reason`.

`working` describes local readiness only. It requires companion-file detection, loading `downloader.yaml`, creating or opening storage and the SQLite database, initializing the manager, and inspecting local free capacity. This startup check does not execute the downloader CLI or contact Hugging Face.

The WebUI shows the Download tab when at least one router node reports `enabled: true`. Enabled nodes remain selectable when initialization fails, and the selected node's router-provided reason is shown while download operations are disabled. The tab is hidden when every reporting node explicitly disables downloading. In a mixed cluster, the initial selection prefers an enabled working node and otherwise selects the first enabled failed node so its diagnostic is visible.

## Hugging Face search

The Download tab searches Hugging Face model repositories through the Hub API. Search supports free text, author, tags and filters, pipeline type, parameter range, application, gated status, inference availability, inference provider, training dataset, sorting, and paged results.

Opening a result resolves the selected revision to a commit and lists repository files, sizes, LFS hashes when present, license metadata, gated status, and repository security status. A Hugging Face token in `downloader.yaml` is optional for public repositories and required when the account must access a private or gated repository. Without a configured token the downloader uses `HF_TOKEN` or `HUGGING_FACE_HUB_TOKEN` from its environment.

`huggingface.endpoint` (or `HF_ENDPOINT`) points the downloader at a Hub mirror. It must use https; plain http is accepted only for a mirror on the same machine. The downloader honours `HTTPS_PROXY`, `HTTP_PROXY` and `NO_PROXY` for both API calls and file transfers.

On Windows nodes, files whose names Windows cannot store (for example names containing `:` or `?`, reserved device names such as `CON`, or names ending in a dot or space) are listed as not downloadable instead of breaking the whole repository. Windows and macOS nodes also refuse plans with two files that differ only by letter case. Linux nodes keep every name their file system accepts.

Hugging Face documents the underlying search and download interfaces in [Search the Hub](https://huggingface.co/docs/huggingface_hub/en/guides/search) and the [`hf` CLI guide](https://huggingface.co/docs/huggingface_hub/en/guides/cli).

## Download planning and jobs

A plan can use automatic file selection or an explicit file list. Before starting, the downloader checks the selected revision, file sizes, available storage, unsafe repository status, gated access, and replacement conflicts that require confirmation.

Jobs are queued and can be inspected, paused, resumed, or cancelled. Job and per-file concurrency, retries, and timeouts come from `downloader.yaml`. The downloader transfers files itself over HTTPS with ranged resume, verifies each file against its Hugging Face LFS SHA-256 (or git blob id for small files), and indexes completed files with hashes and origin metadata in the local artifact library.

Transfers survive unreliable networks:

- Progress is reported while a file downloads, so the WebUI shows bytes, speed and remaining time.
- A connection that delivers no data for `downloads.stall_timeout` (default 60s) is abandoned and resumed from the last byte.
- Failures back off exponentially and honour `Retry-After`. `downloads.retry_limit` counts only attempts that made no progress, so a flaky link that keeps delivering data is never given up on.
- Jobs interrupted by a crash or restart resume from their staged bytes when the downloader starts again.
- Starting the same download twice joins the job already in progress.
- Cancelling a job deletes its partial files.

The library can be rescanned after files change outside the downloader. Indexed artifacts are available to portable `.kcpps` resolution. See [KCPPS Sharing](KCPPS-Sharing).

The router's backend download operation installs one target in Kobold mode and three targets in `llama_sdcpp` mode: llama-server, sd-server, and whisper-server. whisper.cpp release archives are normalized so the executable and required runtime libraries are installed together. Direct sources can be pinned with `updates.whispercpp_binary_sha256`; repository sources default to the official whisper.cpp releases and support `updates.whispercpp_asset_glob`.

## Command-line use

```text
tensor-router-downloader download REPO FILE... --config downloader.yaml --yes
tensor-router-downloader download REPO --all --config downloader.yaml --yes
```

Use `--revision` to select a repository revision. Without `--yes`, the command prints the plan and asks for confirmation.

The downloader uses `downloader.yaml`, documented in [Configuration](Configuration). When logging is enabled, it writes `downloader.log` relative to that configuration file.
