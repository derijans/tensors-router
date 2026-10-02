import { elementTarget } from "../dom";
import { elements } from "../elements";
import { selectLoadCapture } from "../load-captures";
import { selectLoadError } from "../load-errors";
import { selectNode } from "../nodes-state";
import { attentionItemAt, changeOverviewPeriod, refreshNodeSnapshot, setOverviewActive } from "../overview/overview";
import type { AttentionTarget } from "../overview/overview-data";
import { activateTab, onTabActivation } from "../shell/navigation";
import { state } from "../state";
import { runTask } from "../tasks";
import type { OverviewPeriod } from "../types";
import { bindRuntimeActions } from "./runtime-action-bindings";

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
  elements.overviewAttention.addEventListener("click", event => {
    const index = elementTarget(event)?.closest<HTMLElement>("[data-attention-index]")?.dataset.attentionIndex;
    const item = index === undefined ? undefined : attentionItemAt(Number(index));
    if (item) {
      openAttentionTarget(item.target);
    }
  });
  elements.overviewNodes.addEventListener("click", event => {
    const nodeID = elementTarget(event)?.closest<HTMLElement>("[data-node-open]")?.dataset.nodeOpen;
    if (nodeID) {
      openNode(nodeID);
    }
  });
  bindRuntimeActions(elements.overviewNodes, refreshNodeSnapshot);
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
