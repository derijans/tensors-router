# Downloader

The downloader is a separate companion executable. The router exposes its management routes when `downloader.enabled` is `true` and the executable is available.

If `downloader.binary_location` is empty, the router looks for the companion beside its own executable. A configured relative path is resolved from the router configuration directory.

## Hugging Face search

The Download tab searches Hugging Face model repositories through the Hub API. Search supports free text, author, tags and filters, pipeline type, parameter range, application, gated status, inference availability, inference provider, training dataset, sorting, and paged results.

Opening a result resolves the selected revision to a commit and lists repository files, sizes, LFS hashes when present, license metadata, gated status, and repository security status. A Hugging Face token in `downloader.yaml` is optional for public repositories and required when the account must access a private or gated repository.

Hugging Face documents the underlying search and download interfaces in [Search the Hub](https://huggingface.co/docs/huggingface_hub/en/guides/search) and the [`hf` CLI guide](https://huggingface.co/docs/huggingface_hub/en/guides/cli).

## Download planning and jobs

A plan can use automatic file selection or an explicit file list. Before starting, the downloader checks the selected revision, file sizes, available storage, unsafe repository status, gated access, and replacement conflicts that require confirmation.

Jobs are queued and can be inspected, paused, resumed, or cancelled. Job and per-file concurrency, retries, and request timeouts come from `downloader.yaml`. Downloads use the official Hugging Face CLI, and completed files are indexed with hashes and origin metadata in the local artifact library.

The library can be rescanned after files change outside the downloader. Indexed artifacts are available to portable `.kcpps` resolution. See [KCPPS Sharing](KCPPS-Sharing).

## Command-line use

```text
tensor-router-downloader download REPO FILE... --config downloader.yaml --yes
tensor-router-downloader download REPO --all --config downloader.yaml --yes
```

Use `--revision` to select a repository revision. Without `--yes`, the command prints the plan and asks for confirmation.

The downloader uses `downloader.yaml`, documented in [Configuration](Configuration). When logging is enabled, it writes `downloader.log` relative to that configuration file.
