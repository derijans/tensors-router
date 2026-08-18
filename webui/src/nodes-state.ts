import { cancelNodeBackendInitialization, getNodeState, initializeNodeBackend, unloadNodeRuntime } from "./api";
import { closestElement } from "./dom";
import { elements } from "./elements";
import { state } from "./state";
import type { BackendInitializationJob, BackendInitializationRequest, NodeInventory, NodeRuntimeSlice, NodeStateBackend } from "./types";
import { escapeAttribute, escapeHTML } from "./utils";
import { nodeStatePanelID, renderNodeCard, renderNodeStateSnapshot } from "./node-state-view";

const pollIntervalMilliseconds = 1000;

export function renderNodesPanel(): void {
  const nodes = state.inventory?.nodes ?? [];
  reconcileNodeStateSelection(nodes);
  elements.nodeCount.textContent = `${nodes.length} node${nodes.length === 1 ? "" : "s"}`;
  elements.nodesScanNotices.innerHTML = scanNotices(nodes);
  elements.nodesGrid.innerHTML = nodes.map(node => {
    const expanded = state.nodes.expanded.includes(node.node_id);
    return `${renderNodeCard(node, expanded)}${expanded ? nodeStatePanel(node.node_id) : ""}`;
  }).join("");
}

function scanNotices(nodes: NodeInventory[]): string {
  return nodes.filter(node => node.error)
    .map(node => `<div class="inventory-notice error-text">${escapeHTML(node.node_id || node.node_url || "unknown node")}: ${escapeHTML(node.error || "scan failed")}</div>`)
    .join("");
}

export function handleNodesClick(event: Event): void {
  const initializeButton = closestElement(event.target, "[data-node-backend-init]", HTMLButtonElement);
  if (initializeButton) {
    void initializeSelectedBackend(initializeButton.dataset.nodeId || "", initializeButton.dataset.backendId || "", initializeButton.dataset.profile);
    return;
  }
  const cancelInitializationButton = closestElement(event.target, "[data-node-backend-init-cancel]", HTMLButtonElement);
  if (cancelInitializationButton) {
    void cancelSelectedBackendInitialization(cancelInitializationButton.dataset.nodeId || "", cancelInitializationButton.dataset.backendId || "");
    return;
  }
  const unloadButton = closestElement(event.target, "[data-node-unload]", HTMLButtonElement);
  if (unloadButton) {
    const generation = Number(unloadButton.dataset.generation);
    if (Number.isSafeInteger(generation) && generation > 0) {
      void unloadSelectedRuntime(
        unloadButton.dataset.nodeId || "",
        unloadButton.dataset.backendId || "",
        unloadButton.dataset.runtimeId || "",
        generation
      );
    }
    return;
  }
  const closeButton = closestElement(event.target, "[data-node-close]", HTMLButtonElement);
  if (closeButton) {
    collapseNode(closeButton.dataset.nodeId || "");
    return;
  }
  const nodeButton = closestElement(event.target, "[data-node-select]", HTMLButtonElement);
  if (nodeButton?.dataset.nodeSelect) {
    selectNode(nodeButton.dataset.nodeSelect);
  }
}

export function selectNode(nodeID: string): void {
  if (state.nodes.expanded.includes(nodeID)) {
    collapseNode(nodeID);
    return;
  }
  state.nodes.expanded = [...state.nodes.expanded, nodeID];
  const created = defaultNodeSlice();
  created.loading = true;
  state.nodes.byNode[nodeID] = created;
  renderNodesPanel();
  if (state.activeTab === "nodes") {
    void pollNode(nodeID, created.pollGeneration);
  }
}

export function collapseNode(nodeID: string): void {
  if (!nodeID) {
    return;
  }
  invalidatePolling(nodeID);
  state.nodes.expanded = state.nodes.expanded.filter(id => id !== nodeID);
  delete state.nodes.byNode[nodeID];
  renderNodesPanel();
}

export function reconcileNodeStateSelection(nodes: NodeInventory[]): void {
  const present = new Set(nodes.map(node => node.node_id));
  const stale = state.nodes.expanded.filter(id => !present.has(id));
  if (stale.length === 0) {
    return;
  }
  for (const nodeID of stale) {
    invalidatePolling(nodeID);
    delete state.nodes.byNode[nodeID];
  }
  state.nodes.expanded = state.nodes.expanded.filter(id => present.has(id));
}

export function setNodesTabActive(active: boolean): void {
  invalidateAllPolling();
  if (!active) {
    return;
  }
  for (const nodeID of state.nodes.expanded) {
    const current = slice(nodeID);
    if (current) {
      current.loading = current.snapshot === null;
    }
  }
  renderNodesPanel();
  for (const nodeID of state.nodes.expanded) {
    const current = slice(nodeID);
    if (current) {
      void pollNode(nodeID, current.pollGeneration);
    }
  }
}

export function stopNodeStatePolling(): void {
  invalidateAllPolling();
}

export function nodeUnloadKey(backendID: string, runtimeID: string): string {
  return `${backendID} ${runtimeID}`;
}

export function nodeBackendActionKey(action: "init" | "cancel", backendID: string): string {
  return `${action} ${backendID}`;
}

export async function pollNode(nodeID: string, generation: number, clearErrorOnSuccess = true): Promise<void> {
  if (!pollingCurrent(nodeID, generation)) {
    return;
  }
  try {
    const snapshot = await getNodeState(nodeID);
    if (!pollingCurrent(nodeID, generation)) {
      return;
    }
    const current = slice(nodeID);
    if (!current) {
      return;
    }
    const changed = JSON.stringify(current.snapshot) !== JSON.stringify(snapshot) || current.loading || (clearErrorOnSuccess && current.error !== "");
    current.snapshot = snapshot;
    current.loading = false;
    if (clearErrorOnSuccess) {
      current.error = "";
    }
    if (changed) {
      renderNodePanel(nodeID);
    }
  } catch (error) {
    if (!pollingCurrent(nodeID, generation)) {
      return;
    }
    const current = slice(nodeID);
    if (!current) {
      return;
    }
    current.loading = false;
    current.error = errorMessage(error);
    renderNodePanel(nodeID);
  } finally {
    const current = slice(nodeID);
    if (current && pollingCurrent(nodeID, generation)) {
      current.pollTimer = window.setTimeout(() => {
        const latest = slice(nodeID);
        if (latest) {
          latest.pollTimer = null;
        }
        void pollNode(nodeID, generation);
      }, pollIntervalMilliseconds);
    }
  }
}

export async function unloadSelectedRuntime(nodeID: string, backendID: string, runtimeID: string, generation: number): Promise<void> {
  const current = slice(nodeID);
  if (!nodeID || !backendID || !runtimeID || !current || current.pendingUnload) {
    return;
  }
  invalidatePolling(nodeID);
  const pollGeneration = current.pollGeneration;
  const pendingUnload = nodeUnloadKey(backendID, runtimeID);
  current.pendingUnload = pendingUnload;
  current.error = "";
  renderNodePanel(nodeID);
  let succeeded = false;
  try {
    await unloadNodeRuntime({
      node_id: nodeID,
      backend_id: backendID,
      runtime_id: runtimeID,
      expected_generation: generation
    });
    succeeded = true;
  } catch (error) {
    if (pollingCurrent(nodeID, pollGeneration)) {
      const active = slice(nodeID);
      if (active) {
        active.error = errorMessage(error);
      }
    }
  } finally {
    const after = slice(nodeID);
    if (after && after.pendingUnload === pendingUnload) {
      after.pendingUnload = "";
      if (state.activeTab === "nodes" && state.nodes.expanded.includes(nodeID)) {
        renderNodePanel(nodeID);
      }
    }
    if (pollingCurrent(nodeID, pollGeneration)) {
      await pollNode(nodeID, pollGeneration, succeeded);
    }
  }
}

export async function initializeSelectedBackend(nodeID: string, backendID: string, profile?: string): Promise<void> {
  await runBackendInitializationAction("init", nodeID, backendID, profile);
}

export async function cancelSelectedBackendInitialization(nodeID: string, backendID: string): Promise<void> {
  await runBackendInitializationAction("cancel", nodeID, backendID);
}

async function runBackendInitializationAction(action: "init" | "cancel", nodeID: string, backendID: string, profile?: string): Promise<void> {
  const current = slice(nodeID);
  if (!nodeID || !backendID || !current || current.pendingBackendAction) {
    return;
  }
  invalidatePolling(nodeID);
  const pollGeneration = current.pollGeneration;
  const pendingAction = nodeBackendActionKey(action, backendID);
  current.pendingBackendAction = pendingAction;
  current.error = "";
  renderNodePanel(nodeID);
  let succeeded = false;
  try {
    const request: BackendInitializationRequest = {node_id: nodeID, backend_id: backendID};
    if (profile) {
      request.profile = profile;
    }
    const job = action === "init"
      ? await initializeNodeBackend(request)
      : await cancelNodeBackendInitialization(request);
    succeeded = true;
    applyBackendInitializationJob(nodeID, job);
  } catch (error) {
    if (pollingCurrent(nodeID, pollGeneration)) {
      const active = slice(nodeID);
      if (active) {
        active.error = errorMessage(error);
      }
    }
  } finally {
    const after = slice(nodeID);
    if (after && after.pendingBackendAction === pendingAction) {
      after.pendingBackendAction = "";
      if (state.activeTab === "nodes" && state.nodes.expanded.includes(nodeID)) {
        renderNodePanel(nodeID);
      }
    }
    if (pollingCurrent(nodeID, pollGeneration)) {
      await pollNode(nodeID, pollGeneration, succeeded);
    }
  }
}

function applyBackendInitializationJob(nodeID: string, job: BackendInitializationJob): void {
  const current = slice(nodeID);
  if (!current?.snapshot || current.snapshot.node_id !== nodeID) {
    return;
  }
  const backend = current.snapshot.backends.find(candidate => candidate.id === job.backend_id);
  if (!backend) {
    return;
  }
  backend.lifecycle_state = lifecycleStateForJob(backend, job);
  backend.initialization_job_id = job.job_id;
  backend.initialization_bytes = job.completed_bytes;
  backend.initialization_total_bytes = job.total_bytes;
  backend.retryable = Boolean(job.retryable);
  assignOptionalBackendValue(backend, "initialization_phase", job.phase);
  assignOptionalBackendValue(backend, "error", job.error);
  assignOptionalBackendValue(backend, "selected_profile", job.selected_profile);
  assignOptionalBackendValue(backend, "detected_profile", job.detected_profile);
}

function lifecycleStateForJob(backend: NodeStateBackend, job: BackendInitializationJob): NodeStateBackend["lifecycle_state"] {
  if (job.state === "completed") {
    return "ready";
  }
  if (job.state === "failed") {
    return "failed";
  }
  if (job.state === "cancelled") {
    return backend.runtime_version ? "ready" : "needs_init";
  }
  return "initializing";
}

function assignOptionalBackendValue(backend: NodeStateBackend, key: "initialization_phase" | "error" | "selected_profile" | "detected_profile", value: string | undefined): void {
  if (value) {
    backend[key] = value;
    return;
  }
  delete backend[key];
}

function defaultNodeSlice(): NodeRuntimeSlice {
  return {snapshot: null, loading: false, error: "", pollGeneration: 0, pollTimer: null, pendingUnload: "", pendingBackendAction: ""};
}

function slice(nodeID: string): NodeRuntimeSlice | undefined {
  return state.nodes.byNode[nodeID];
}

function invalidatePolling(nodeID: string): void {
  const current = slice(nodeID);
  if (!current) {
    return;
  }
  current.pollGeneration++;
  if (current.pollTimer !== null) {
    window.clearTimeout(current.pollTimer);
    current.pollTimer = null;
  }
}

function invalidateAllPolling(): void {
  for (const nodeID of Object.keys(state.nodes.byNode)) {
    invalidatePolling(nodeID);
  }
}

function pollingCurrent(nodeID: string, generation: number): boolean {
  const current = slice(nodeID);
  return current !== undefined &&
    generation === current.pollGeneration &&
    nodeID !== "" &&
    state.nodes.expanded.includes(nodeID) &&
    state.activeTab === "nodes";
}

function renderNodePanel(nodeID: string): void {
  const panel = document.getElementById(nodeStatePanelID(nodeID));
  if (panel) {
    panel.outerHTML = nodeStatePanel(nodeID);
  }
}

function nodeStatePanel(nodeID: string): string {
  const current = slice(nodeID) ?? defaultNodeSlice();
  return `
    <section id="${escapeAttribute(nodeStatePanelID(nodeID))}" class="node-state-panel" aria-label="Runtime state for ${escapeAttribute(nodeID)}">
      <div class="node-state-header">
        <h3>Runtime state - ${escapeHTML(nodeID)}</h3>
        <button type="button" data-node-close data-node-id="${escapeAttribute(nodeID)}">Close</button>
      </div>
      ${current.loading && !current.snapshot ? `<p class="muted node-state-message">Loading runtime state...</p>` : ""}
      ${current.error ? `<div class="error-text node-state-message" role="alert">${escapeHTML(current.error)}</div>` : ""}
      ${current.snapshot ? renderNodeStateSnapshot(nodeID, current.snapshot, current.pendingUnload, current.pendingBackendAction) : ""}
    </section>
  `;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
