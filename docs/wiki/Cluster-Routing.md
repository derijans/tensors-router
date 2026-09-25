# Cluster Routing

## Roles

- `standalone` keeps routing and inference on one node.
- `master` combines local and remote model records and accepts client requests.
- `slave` advertises local models and accepts authenticated worker requests from the master.

Use a unique `cluster.node_id` for every node and the same non-placeholder `cluster.token` across the cluster.

A slave requires `cluster.master_url` and its own reachable `cluster.public_url`. A master accepts registration only from URLs listed in `cluster.slave_urls`. Masters also poll configured slave URLs during startup and health intervals.

## Registration and health

Slaves register their snapshots with the master at startup and on the synchronization interval. The master polls configured slaves and marks a node unhealthy when a poll or its authorization fails. Every model on an unhealthy node is unavailable until the next successful poll.

On the local node, text, embedding, multimodal, speech, and music routes use text-side readiness. Image and video routes use image-side readiness. Split transcription uses Whisper `/health`; Kobold capability checks cover its shared process. A local model is selected only when its required lane is available. Remote nodes are tracked by whole-node health, not per lane.

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
`cluster.scheduling_backend_depth` requests reach the backend at once.

This is the intended flow, with the default depth of 2. An owner receives 10
requests for model A:

1. 2 go straight to A's backend: one generating, one waiting inside the backend,
   so A never idles between jobs.
2. The other 8 are held in the router and judged for lending at once.
3. A helper that pays off gets 2 of them in flight when its predicted time per job
   is at most the owner's, otherwise 1.
4. The rest stay held. A is always topped back up to 2 from the held requests.
5. Every result that comes back, from A or from the helper, triggers the next
   decision: top A off, or give the helper more.

Only held requests can be lent. The two in A's pipe never move.

The node that first received the requests owns them and keeps the client
connections for their whole life. It lends surplus backlog; it never transfers
custody.

### Who decides

The master decides; the owner executes. The master pushes the current links to
every slave. An owner reports every queue event of a linked model to the master
(`POST /router/v1/node/offload/event`): each new held request and each finished
job, its own or a lent one. Events for the same model are coalesced, so a burst
costs at most one decision in flight and one queued behind it.

On every event, and on a `cluster.scheduling_refresh_interval` tick as a fallback,
the master polls every node for its queue, whether it accepts borrowed work, how
long it has been idle, and the cost coefficients that node fitted from its own
analytics. For each owner with held work it considers only the helpers that owner
links to, and answers with a lease naming the helper and how many requests that
helper may hold at once, or with no lease. The owner lends held requests up to
the lease's slot count and stops when the answer is no lease. A request already
lent is never recalled. The master rewrites each lent request to the helper's own
model ID.

### Pricing

The master compares what the owner needs to drain its backlog alone against what
the helper needs to hand back the first lent job, model load included. A load is
paid once and amortises over every job that then flows through the slot, which is
why a deep backlog justifies a switch that a shallow one does not.

A model has its own cost fit once it has `cluster.scheduling_min_samples` measured
requests of varied size inside `cluster.scheduling_sample_window`. A helper with
no fit of its own is priced with the owner's fit for the same work, and switches
to its own as soon as it has one. Every successful load counts as load-cost data,
including loads that were never followed by a generation, so a helper that was
only ever loaded still has a switch cost.

### Probes

A helper that the cost rule does not choose still gets a one-request probe once
it has been idle for `cluster.offload_probe_idle` (default `5s`), as long as it is
accepting borrowed work, its link allows it to take the work, and, for text, its
context window holds the work. Probes happen even when the owner is faster or
neither side is priced yet, so every linked pair gradually gathers the samples its
own fit needs. When a probe is held back only because the helper has not idled
long enough yet, the master decides again the moment it has, without waiting for
the owner's next event.

### A helper's own work

When a node that is holding borrowed work receives a request of its own, it
finishes the borrowed jobs already inside its backend (up to the depth, so up to 2
by default), hands back any borrowed request it has not started with a `409
offload_returned` response, serves its own request, and stops accepting borrowed
work until its own queue is empty. The owner re-queues whatever came back and runs
it. A return is not recorded as a failed request. Each time a node's own request
waits behind borrowed work, the wait is written to the decision log.

### Decision log

Every lending decision is written to the `offload_decisions` table in the router
database, pruned after `analytics.raw_retention`:

- `plan` rows on the master, one per owner and helper pair per decision: the
  trigger (`enqueued`, `completed`, `borrowed_completed`, `probe_due`, `tick`), held and backlog counts, the owner's drain time, the helper's switch and
  service time, whether the service time came from the helper's own fit or the
  owner's (`helper` or `owner_fallback`), the helper's idle time, the outcome
  (`granted`, `probe`, `skipped`) and the reason for a skip or a probe
  (`owner_unpriced`, `switch_unpriced`, `service_unpriced`, `cost_rejected`,
  `outranked`, `helper_not_accepting`, `helper_claimed`, `load_forbidden`,
  `context_too_small`), and the slots granted.
- `dispatch` rows on the owner: `lease_updated`, `lease_cleared`, `lent`, and
  `returned`, with the slots and how many requests were out at that moment.
- `helper` rows on the helper: `native_waited_behind_borrowed`, with how many
  borrowed jobs were already in the backend and how long its own request waited
  before reaching the backend, and `borrowed_returned`, with whether the helper
  was already busy when the request arrived or its own work arrived afterwards.

The Nodes page lists each node's held requests: requests the router holds before
passing them to a backend, and requests currently lent to a helper. Requests that
go straight to a backend are listed only under active requests.

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
has none of its own work running. If work is still running at that moment, it
checks again every `cluster.offload_restore_delay` until the node is quiet. A native request arriving first cancels the
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
