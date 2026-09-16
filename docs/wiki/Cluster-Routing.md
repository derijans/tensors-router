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

## Backlog lending

Routing links let an operator declare that one model may lend queued work to a
model on another node. A link is one-way: the **owner** lends, the **helper**
borrows. The reverse direction is a separate link and is never implied. Links are
chosen by hand in the WebUI and the two models are not required to share a name, a
config hash, or a checkpoint. The common case is one checkpoint that two nodes
configured differently.

Image models and LLM models are linked separately, under
`/router/v1/site/routing-groups` and `/router/v1/site/text-routing-groups`. Both
lanes use the same lending rules below. Each lane keeps its own cost model.

### A request goes to the model it names

Links never change where a request is routed. A request for `cc11-ff` is served by
the node or nodes that hold `cc11-ff`, and replicas with the same public ID keep
their usual rotation. A faster linked model on another node does not attract the
request. To spread one ID across nodes, give the configs the same name instead.

### What is lent

A model that is an owner or a helper in any link is enrolled, and its requests are
queued inside the router instead of being handed straight to the backend. Only
`cluster.scheduling_backend_depth` requests reach the backend at once. With the
default of 2, four requests for an owner mean one running, one waiting inside the
backend, and two waiting in the router. Only those last two can be lent.

The node that first received the requests owns them and keeps the client
connections for their whole life. It lends surplus backlog; it never transfers
custody.

Lending is master mediated. A slave cannot reach another slave, so the master
brokers placement. The master pushes the current links to every slave, polls every
node for its queue depth, whether it is accepting borrowed work, and the cost
coefficients that node fitted from its own analytics. For each owner with queued
work, it considers only the helpers that owner links to. It compares what the owner
needs to drain alone against what an idle helper needs to return the first borrowed
job, model load included, and leases the owner a slot on the best helper. A lease
carries a time to live and is renewed only while it still pays off. The master
rewrites each lent request to the helper's own model ID.

Borrowed work moves one request at a time. The next is sent only once the previous
one completes, so a helper never accumulates a borrowed queue.

When a node that is holding borrowed work receives a request of its own, it
finishes the borrowed job it is running, hands back any borrowed request it has
not started with a `409 offload_returned` response, serves its own request, and
stops accepting borrowed work until its own queue is empty. The owner re-queues
whatever came back and runs it. A return is not recorded as a failed request.

An owner or helper without enough measured requests to be priced is skipped,
controlled by `cluster.scheduling_min_samples` and
`cluster.scheduling_sample_window`. A node is never scheduled on a guess.

### Loading the helper model

Each link has a **load if unloaded** flag. When it is set, a helper that holds a
different model may load the linked model to take lent work, and the load time is
part of the comparison above. When it is cleared, the helper takes lent work only
while the linked model is already loaded. A borrowed request that would still need
a load on arrival is handed back with `409 offload_returned`.

### Reloading the model borrowed work displaced

Serving a borrowed request can load the helper model, which evicts whatever that
helper already had loaded. Nothing restores that on its own: the helper keeps the
linked model until its own next request pays the switch cost again.

Each link also has a **restore after borrow** flag. When it is set, the helper
reloads the model it held before, once `cluster.offload_restore_delay` (default
`1.5s`) passes with no further borrowed work arriving on either lane and the node
has none of its own work running. A native request arriving first cancels the
pending restore instead of competing with it, and a run of consecutive borrowed
requests restores whatever was loaded before the run started, not an intermediate
model.

### Linking models that are not identical

A link asserts that the helper can answer for the owner. The router cannot verify
that. If the two really are different checkpoints, clients receive different output
for the same model ID and nothing flags it.

The picker labels every candidate using data the registry already carries. Same
weights means the model hash matches the anchor. Different weights means it does
not, and linking such a model for the first time requires an explicit
acknowledgement before the links can be saved.

### Upgrading from routing groups

Earlier releases stored symmetric routing groups, where every member could serve
every other member. On upgrade, each pair of members on different nodes becomes two
links, one per direction, with loading allowed and the old restore flag kept on the
link whose helper had it. Lending therefore behaves as before until an operator
clears the direction they do not want.

## LLM lending

LLM links follow every rule above. Two things are specific to text models.

**Eligibility.** A text model may be linked only if it serves requests one at a
time (`parallel` and `vllm.settings.max_number_sequences` both `1` or unset: a
config that serves concurrently is already outside the one-request-per-slot
discipline the queue assumes) and states a context window (`contextsize`, or a
vLLM config's `settings.max_model_length`). A multimodal model links only with
another multimodal model. An ineligible model is listed as a candidate but shown
disabled with the reason, never silently omitted. A linked model whose config
becomes ineligible is skipped at the next lease plan, not just at the next save.

**The context gate.** A helper is leased only if its context window can hold the
mean estimated size of the owner's queued requests. When a queued request is
withdrawn for lending and does not fit the leased helper, the owner runs it itself.
A model with no measured token profile has no size estimate, the same discipline
`cluster.scheduling_min_samples` already applies to cost fitting.

Token counts are estimated, never counted: the router divides the raw request
body size by a bytes-per-token ratio it measures from that model's own traffic,
pulled conservative by the ratio's own measured spread rather than a configured
margin. `cluster.scheduling_context_reserve` is only the floor on how much of the
window is reserved for the answer when the client states no `max_tokens`; the rest
comes from the same measured history.

A streaming (`stream: true`) request is queued but never withdrawn for lending:
the router only buffers a replayable body for a non-streaming request, so a stream
can never be handed to another node mid-flight.

A model with no text link is completely unaffected by any of this: no queue, no
gate, no lending.

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
