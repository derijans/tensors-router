import type { InventoryResponse, LaneKind, Options } from "../../../types";
import { laneMetadata } from "../../../domain/lanes";
import { inventoryOptionDefinition } from "../../../domain/inventory";

export function advancedLaneOptions(inventory: InventoryResponse | null, lane: LaneKind, options: Options): Options {
  const section = laneMetadata[lane].section;
  const result: Options = {};
  for (const [key, value] of Object.entries(options)) {
    const definition = inventoryOptionDefinition(inventory, key);
    if (!definition || definition.section === "runtime" || definition.section === section) {
      result[key] = value;
    }
  }
  return result;
}
