import { state } from "../../state/app-state";
import { defaultNodeSlice, invalidatePolling, nodeExpanded } from "./node-slices";
import { pollNode } from "./nodes-polling";
import { renderNodesPanel } from "./nodes-render";

export function selectNode(nodeID: string): void {
  if (nodeExpanded(nodeID)) {
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
