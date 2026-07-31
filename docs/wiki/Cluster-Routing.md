# Cluster Routing

## Roles

- `standalone` keeps routing and inference on one node.
- `master` combines local and remote model records and accepts client requests.
- `slave` advertises local models and accepts authenticated worker requests from the master.

Use a unique `cluster.node_id` for every node and the same non-placeholder `cluster.token` across the cluster.

A slave requires `cluster.master_url` and its own reachable `cluster.public_url`. A master accepts registration only from URLs listed in `cluster.slave_urls`. Masters also poll configured slave URLs during startup and health intervals.

## Registration and health

Slaves register their snapshots with the master at startup and on the synchronization interval. The master polls configured slaves and records lane-specific health.

Text, embedding, multimodal, speech, and music routes use text-side readiness. Image and video routes use image-side readiness. Split transcription uses Whisper `/health`; Kobold capability checks cover its shared process. A node is selected only when its required lane is available.

Selector-less STT scheduling uses authenticated runtime status from current nodes. It prefers a loaded local STT configuration, then a loaded healthy remote configuration with the shortest whole-node active-plus-queued workload, then a wholly idle capable node. If every node is busy, the master queues locally when compatible, otherwise it chooses the shortest remote whole-node queue. Equal candidates rotate round-robin, and the selected route is reserved before its configuration can load. Nodes without runtime-status support remain available for explicit-model requests but do not participate in automatic selection.

## Model identity

Cluster records contain public and local IDs, node identity, source, configuration and asset hashes, backend family, capabilities, and availability.

Models with the same public ID and identical hashes are presented as one model. When hashes differ, the master adds a stable numeric suffix to the conflicting public ID.

Before forwarding, the master replaces the public ID with the selected slave's local ID. It rewrites model IDs in supported responses back to the public value.

## Asset handling

`cluster.store_dir` stores registry state, recipes, benchmarks, configuration hashes, and analytics data when enabled.

`models.shared_dir` provides a shared asset location when configured. Model asset APIs resolve references by verified hashes, support bounded transfers, and record binding or substitution results.

File roots restrict the inventory that management operations can expose. Asset paths received from remote nodes are not trusted without the configured node relationship and hash validation.

## Split recipes

A master recipe can select components from different nodes. The master routes each request to the node that owns the required component and backend family.

Explicit recipe loading prepares the selected components. Request routing still checks node availability and keeps public model identity stable.

## Routing-only master

To keep inference off the master:

1. Create an empty directory for `models.config_dir`.
2. Leave `models.startup_model` empty.
3. Set `backend.mode: "llama_sdcpp"` so all local backend processes remain lazy.
4. Set `cluster.role: "master"` and list each permitted slave URL.
5. Do not create recipes with components assigned to the master.

The master still handles registry synchronization, authentication, routing, management operations, and any enabled analytics.
