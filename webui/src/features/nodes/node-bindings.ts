import { closestElement } from "../../lib/dom";
import { onTabActivation } from "../shell/navigation";
import { refreshInventory } from "../shell/refresh";
import { runTask } from "../shell/tasks";
import { collapseNode, selectNode } from "./nodes";
import { applyLaunchOptionsTickedInPanel, cancelSelectedBackendInitialization, initializeSelectedBackend, unloadSelectedRuntime } from "./nodes-actions";
import { nodesElements } from "./nodes-elements";
import { pollNodeOnce, setNodesTabActive } from "./nodes-polling";
import { bindRuntimeActions } from "./runtime-action-bindings";

type NodeClickHandler = (button: HTMLButtonElement) => void;

const nodeClickHandlers: readonly (readonly [selector: string, handler: NodeClickHandler])[] = [
  ["[data-node-backend-init]", button => void initializeSelectedBackend(button.dataset.nodeId || "", button.dataset.backendId || "", button.dataset.profile)],
  ["[data-node-backend-init-cancel]", button => void cancelSelectedBackendInitialization(button.dataset.nodeId || "", button.dataset.backendId || "")],
  ["[data-node-backend-launch-apply]", button => void applyLaunchOptionsTickedInPanel(button)],
  ["[data-node-unload]", unloadRuntimeFromButton],
  ["[data-node-close]", button => collapseNode(button.dataset.nodeId || "")],
  ["[data-node-select]", button => button.dataset.nodeSelect && selectNode(button.dataset.nodeSelect)]
];

export function bindNodes(): void {
  onTabActivation(tab => setNodesTabActive(tab === "nodes"));
  nodesElements.nodesGrid.addEventListener("click", handleNodesClick);
  nodesElements.nodesDetail.addEventListener("click", handleNodesClick);
  nodesElements.nodesRefreshButton.addEventListener("click", () => runTask(() => refreshInventory(false), "nodes-refresh", "nodes", "Refreshing nodes…"));
  bindRuntimeActions(nodesElements.nodesGrid, pollNodeOnce);
}

function handleNodesClick(event: Event): void {
  for (const [selector, handler] of nodeClickHandlers) {
    const button = closestElement(event.target, selector, HTMLButtonElement);
    if (button) {
      handler(button);
      return;
    }
  }
}

function unloadRuntimeFromButton(button: HTMLButtonElement): void {
  const generation = Number(button.dataset.generation);
  if (Number.isSafeInteger(generation) && generation > 0) {
    void unloadSelectedRuntime(button.dataset.nodeId || "", button.dataset.backendId || "", button.dataset.runtimeId || "", generation);
  }
}
