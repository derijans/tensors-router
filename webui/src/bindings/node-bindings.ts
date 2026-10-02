import { refreshInventory } from "../app-data";
import { elements } from "../elements";
import { handleNodesClick, pollNodeOnce, setNodesTabActive } from "../nodes-state";
import { onTabActivation } from "../shell/navigation";
import { runTask } from "../tasks";
import { bindRuntimeActions } from "./runtime-action-bindings";

export function bindNodes(): void {
  onTabActivation(tab => setNodesTabActive(tab === "nodes"));
  elements.nodesGrid.addEventListener("click", handleNodesClick);
  elements.nodesDetail.addEventListener("click", handleNodesClick);
  elements.nodesRefreshButton.addEventListener("click", () => runTask(() => refreshInventory(false), "nodes-refresh", "nodes", "Refreshing nodes…"));
  bindRuntimeActions(elements.nodesGrid, pollNodeOnce);
}
