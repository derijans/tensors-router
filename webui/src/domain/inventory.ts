import { groupBy } from "../lib/collections";
import type { CookComponent, InventoryResponse, NodeInventory, OptionDefinition } from "../types";

export function inventoryOptionDefinitions(inventory: InventoryResponse | null): OptionDefinition[] {
  return [
    ...(inventory?.option_catalog ?? []),
    ...(inventory?.observed_options ?? [])
  ];
}

export function inventoryOptionDefinition(inventory: InventoryResponse | null, key: string): OptionDefinition | undefined {
  return inventoryOptionDefinitions(inventory).find(option => option.key === key);
}

export function inventoryNode(inventory: InventoryResponse | null, nodeID: string): NodeInventory | undefined {
  return (inventory?.nodes ?? []).find(node => node.node_id === nodeID);
}

export function componentsByNode(inventory: InventoryResponse | null, components: CookComponent[]): Map<string, CookComponent[]> {
  return groupBy(components, component => component.node_id || inventory?.node_id || "local");
}
