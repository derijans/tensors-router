import { elementTarget } from "../../lib/dom";
import { selectLoadCapture } from "../insights/load-captures/load-captures";
import { selectLoadError } from "../insights/load-errors/load-errors";
import { selectNode } from "../nodes/nodes";
import { attentionItemAt, changeOverviewPeriod, refreshNodeSnapshot, setOverviewActive } from "./overview";
import type { AttentionTarget } from "./overview-data";
import { activateTab, onTabActivation } from "../shell/navigation";
import { state } from "../../state/app-state";
import { runTask } from "../shell/tasks";
import type { OverviewPeriod } from "../../types";
import { bindRuntimeActions } from "../nodes/runtime-action-bindings";
import { overviewElements } from "./overview-elements";

export function bindOverview(): void {
  onTabActivation(tab => setOverviewActive(tab === "overview"));
  document.querySelectorAll<HTMLButtonElement>("[data-overview-period]").forEach(button => {
    button.addEventListener("click", () => {
      const period = button.dataset.overviewPeriod;
      if (isOverviewPeriod(period)) {
        runTask(() => changeOverviewPeriod(period), "overview-period", "overview", "Loading activity…");
      }
    });
  });
  overviewElements.overviewAttention.addEventListener("click", event => {
    const index = elementTarget(event)?.closest<HTMLElement>("[data-attention-index]")?.dataset.attentionIndex;
    const item = index === undefined ? undefined : attentionItemAt(Number(index));
    if (item) {
      openAttentionTarget(item.target);
    }
  });
  overviewElements.overviewNodes.addEventListener("click", event => {
    const nodeID = elementTarget(event)?.closest<HTMLElement>("[data-node-open]")?.dataset.nodeOpen;
    if (nodeID) {
      openNode(nodeID);
    }
  });
  bindRuntimeActions(overviewElements.overviewNodes, refreshNodeSnapshot);
}

function isOverviewPeriod(value: string | undefined): value is OverviewPeriod {
  return value === "24h" || value === "7d" || value === "30d";
}

function openAttentionTarget(target: AttentionTarget): void {
  switch (target.kind) {
    case "load-error":
      activateTab("load-errors");
      selectLoadError(target.id);
      break;
    case "load-capture":
      activateTab("load-captures");
      runTask(() => selectLoadCapture(target.nodeID, target.attemptID), `load-capture-${target.attemptID}`, "load-captures", "Loading capture…");
      break;
    case "node":
      openNode(target.nodeID);
      break;
    case "analytics":
      activateTab("analytics");
      break;
  }
}

function openNode(nodeID: string): void {
  activateTab("nodes");
  if (!state.nodes.expanded.includes(nodeID)) {
    selectNode(nodeID);
  }
}
