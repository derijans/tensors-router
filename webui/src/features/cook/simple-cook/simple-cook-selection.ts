import { allNodeFiles, nodeByID } from "../../../state/inventory-selectors";
import { state } from "../../../state/app-state";
import type { FieldRenderContext, Model, NodeInventory } from "../../../types";

export function selectedNode(): NodeInventory | null {
  return nodeByID(state.simpleCook.nodeID) ?? (state.inventory?.nodes ?? [])[0] ?? null;
}

export function selectedConfig(): Model | null {
  const node = selectedNode();
  return (node?.models ?? []).find(model => model.local_id === state.simpleCook.configID) ?? null;
}

export function fieldRenderContext(): FieldRenderContext {
  const node = selectedNode();
  const nodeID = node?.node_id || "";
  const nodeModels = node?.models ?? [];
  return {
    node,
    nodeFiles: allNodeFiles().filter(file => file.node_id === nodeID),
    nodeModels,
    otherNodeModels: nodeModels.filter(model => model.local_id !== state.simpleCook.configID),
    comparableBySection: new Map<string, Model[]>(),
    currentFields: state.simpleCook.fields
  };
}
