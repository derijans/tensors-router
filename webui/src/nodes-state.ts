import { getNodeState, unloadNodeRuntime } from "./api";
import { closestElement } from "./dom";
import { elements } from "./elements";
import { state } from "./state";
import type { NodeInventory } from "./types";
import { escapeAttribute, escapeHTML } from "./utils";
import { renderNodeCard, renderNodeStateSnapshot } from "./node-state-view";

const pollIntervalMilliseconds = 1000;

export function renderNodesPanel(): void {
  const nodes = state.inventory?.nodes ?? [];
  reconcileNodeStateSelection(nodes);
  elements.nodeCount.textContent = `${nodes.length} nodes`;
  elements.nodesGrid.innerHTML = nodes.map(node => {
    const selected = node.node_id === state.nodes.selectedNodeID;
    return `${renderNodeCard(node, selected)}${selected ? nodeStatePanel() : ""}`;
  }).join("");
}

export function handleNodesClick(event: Event): void {
  const unloadButton = closestElement(event.target, "[data-node-unload]", HTMLButtonElement);
  if (unloadButton) {
    const generation = Number(unloadButton.dataset.generation);
    if (Number.isSafeInteger(generation) && generation > 0) {
      void unloadSelectedRuntime(
        unloadButton.dataset.backendId || "",
        unloadButton.dataset.runtimeId || "",
        generation
      );
    }
    return;
  }
  if (closestElement(event.target, "[data-node-close]", HTMLButtonElement)) {
    collapseNodeState();
    return;
  }
  const nodeButton = closestElement(event.target, "[data-node-select]", HTMLButtonElement);
  if (nodeButton?.dataset.nodeSelect) {
    selectNode(nodeButton.dataset.nodeSelect);
  }
}

export function selectNode(nodeID: string): void {
  if (state.nodes.selectedNodeID === nodeID) {
    collapseNodeState();
    return;
  }
  resetPolling(nodeID);
  state.nodes.snapshot = null;
  state.nodes.loading = true;
  state.nodes.error = "";
  state.nodes.pendingUnload = "";
  renderNodesPanel();
  if (state.activeTab === "nodes") {
    void pollSelectedNode(state.nodes.pollGeneration);
  }
}

export function setNodesTabActive(active: boolean): void {
  invalidatePolling();
  if (!active || !state.nodes.selectedNodeID) {
    return;
  }
  state.nodes.loading = state.nodes.snapshot === null;
  renderSelectedStatePanel();
  void pollSelectedNode(state.nodes.pollGeneration);
}

export function stopNodeStatePolling(): void {
  invalidatePolling();
}

export function collapseNodeState(): void {
  invalidatePolling();
  state.nodes.selectedNodeID = "";
  state.nodes.snapshot = null;
  state.nodes.loading = false;
  state.nodes.error = "";
  state.nodes.pendingUnload = "";
  renderNodesPanel();
}

export function reconcileNodeStateSelection(nodes: NodeInventory[]): void {
  const selectedNodeID = state.nodes.selectedNodeID;
  if (!selectedNodeID || nodes.some(node => node.node_id === selectedNodeID)) {
    return;
  }
  invalidatePolling();
  state.nodes.selectedNodeID = "";
  state.nodes.snapshot = null;
  state.nodes.loading = false;
  state.nodes.error = "";
  state.nodes.pendingUnload = "";
}

export function nodeUnloadKey(backendID: string, runtimeID: string): string {
  return `${backendID}\u0000${runtimeID}`;
}

export async function pollSelectedNode(generation: number, clearErrorOnSuccess = true): Promise<void> {
  const nodeID = state.nodes.selectedNodeID;
  if (!pollingCurrent(generation, nodeID)) {
    return;
  }
  try {
    const snapshot = await getNodeState(nodeID);
    if (!pollingCurrent(generation, nodeID)) {
      return;
    }
    const changed = JSON.stringify(state.nodes.snapshot) !== JSON.stringify(snapshot) || state.nodes.loading || (clearErrorOnSuccess && state.nodes.error !== "");
    state.nodes.snapshot = snapshot;
    state.nodes.loading = false;
    if (clearErrorOnSuccess) {
      state.nodes.error = "";
    }
    if (changed) {
      renderSelectedStatePanel();
    }
  } catch (error) {
    if (!pollingCurrent(generation, nodeID)) {
      return;
    }
    state.nodes.loading = false;
    state.nodes.error = errorMessage(error);
    renderSelectedStatePanel();
  } finally {
    if (pollingCurrent(generation, nodeID)) {
      state.nodes.pollTimer = window.setTimeout(() => {
        state.nodes.pollTimer = null;
        void pollSelectedNode(generation);
      }, pollIntervalMilliseconds);
    }
  }
}

export async function unloadSelectedRuntime(backendID: string, runtimeID: string, generation: number): Promise<void> {
  if (!backendID || !runtimeID || state.nodes.pendingUnload) {
    return;
  }
  const nodeID = state.nodes.selectedNodeID;
  if (!nodeID) {
    return;
  }
  invalidatePolling();
  const pollGeneration = state.nodes.pollGeneration;
  const pendingUnload = nodeUnloadKey(backendID, runtimeID);
  state.nodes.pendingUnload = pendingUnload;
  state.nodes.error = "";
  renderSelectedStatePanel();
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
    if (pollingCurrent(pollGeneration, nodeID)) {
      state.nodes.error = errorMessage(error);
    }
  } finally {
    if (state.nodes.selectedNodeID === nodeID && state.nodes.pendingUnload === pendingUnload) {
      state.nodes.pendingUnload = "";
      if (state.activeTab === "nodes") {
        renderSelectedStatePanel();
      }
    }
    if (pollingCurrent(pollGeneration, nodeID)) {
      await pollSelectedNode(pollGeneration, succeeded);
    }
  }
}

function resetPolling(nodeID: string): void {
  invalidatePolling();
  state.nodes.selectedNodeID = nodeID;
}

function invalidatePolling(): void {
  state.nodes.pollGeneration++;
  if (state.nodes.pollTimer !== null) {
    window.clearTimeout(state.nodes.pollTimer);
    state.nodes.pollTimer = null;
  }
}

function pollingCurrent(generation: number, nodeID: string): boolean {
  return generation === state.nodes.pollGeneration &&
    nodeID !== "" &&
    nodeID === state.nodes.selectedNodeID &&
    state.activeTab === "nodes";
}

function renderSelectedStatePanel(): void {
  const current = document.getElementById("nodeStatePanel");
  if (current) {
    current.outerHTML = nodeStatePanel();
  }
}


function nodeStatePanel(): string {
  const nodeID = state.nodes.selectedNodeID;
  return `
    <section id="nodeStatePanel" class="node-state-panel" aria-label="Runtime state for ${escapeAttribute(nodeID)}">
      <div class="node-state-header">
        <h3>Runtime state - ${escapeHTML(nodeID)}</h3>
        <button type="button" data-node-close>Close</button>
      </div>
      ${state.nodes.loading && !state.nodes.snapshot ? `<p class="muted node-state-message">Loading runtime state...</p>` : ""}
      ${state.nodes.error ? `<div class="error-text node-state-message" role="alert">${escapeHTML(state.nodes.error)}</div>` : ""}
      ${state.nodes.snapshot ? renderNodeStateSnapshot(state.nodes.snapshot, state.nodes.pendingUnload) : ""}
    </section>
  `;
}


function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
