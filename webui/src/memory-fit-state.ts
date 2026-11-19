import { getNodeMemoryFit } from "./api";
import { reportErrorToConsole } from "./console-report";
import { state } from "./state";
import type { NodeInventory, NodeMemoryFit } from "./types";

export async function loadMemoryFit(nodes: readonly NodeInventory[]): Promise<void> {
  const available = nodes.filter(node => node.available && node.node_id);
  const results = await Promise.all(available.map(node => getNodeMemoryFit(node.node_id).catch((error: unknown) => {
    reportErrorToConsole(`memory fit ${node.node_id}`, error);
    return null;
  })));
  const fits: Record<string, NodeMemoryFit> = {};
  for (const fit of results) {
    if (fit) {
      fits[fit.node_id] = fit;
    }
  }
  state.models.memoryFit = fits;
}
