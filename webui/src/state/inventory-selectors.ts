import { state } from "./app-state";
import type { FileRecord, Model, NodeInventory, OptionDefinition } from "../types";
import { inventoryNode, inventoryOptionDefinition, inventoryOptionDefinitions } from "../domain/inventory";

export function allNodeModels(): Model[] {
  return (state.inventory?.nodes ?? []).flatMap(node => node.models ?? []);
}

export function allNodeFiles(): FileRecord[] {
  return (state.inventory?.nodes ?? []).flatMap(node => node.files ?? []);
}

export function allOptionDefinitions(): OptionDefinition[] {
  return inventoryOptionDefinitions(state.inventory);
}

export function optionDefinition(key: string): OptionDefinition | undefined {
  return inventoryOptionDefinition(state.inventory, key);
}

export function nodeByID(nodeID: string): NodeInventory | undefined {
  return inventoryNode(state.inventory, nodeID);
}

export function nodeModelByID(nodeID: string, modelID: string): Model | undefined {
  return (nodeByID(nodeID)?.models ?? []).find(model => model.local_id === modelID || model.public_id === modelID);
}
