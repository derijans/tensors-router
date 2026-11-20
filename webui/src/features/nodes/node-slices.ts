import { state } from "../../state/app-state";
import type { NodeInventory, NodeRuntimeSlice } from "../../types";

export function defaultNodeSlice(): NodeRuntimeSlice {
  return {snapshot: null, loading: false, error: "", pollGeneration: 0, pollTimer: null, pendingUnload: "", pendingBackendAction: ""};
}

export function nodeSlice(nodeID: string): NodeRuntimeSlice | undefined {
  return state.nodes.byNode[nodeID];
}

export function nodeExpanded(nodeID: string): boolean {
  return state.nodes.expanded.includes(nodeID);
}

export function nodePanelVisible(nodeID: string): boolean {
  return state.activeTab === "nodes" && nodeExpanded(nodeID);
}

export function pollingCurrent(nodeID: string, generation: number): boolean {
  return generation === nodeSlice(nodeID)?.pollGeneration && nodeID !== "" && nodePanelVisible(nodeID);
}

export function invalidatePolling(nodeID: string): void {
  const current = nodeSlice(nodeID);
  if (!current) {
    return;
  }
  current.pollGeneration++;
  if (current.pollTimer !== null) {
    window.clearTimeout(current.pollTimer);
    current.pollTimer = null;
  }
}

export function invalidateAllPolling(): void {
  for (const nodeID of Object.keys(state.nodes.byNode)) {
    invalidatePolling(nodeID);
  }
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
