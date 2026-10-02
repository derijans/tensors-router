import { unloadNodeRuntime } from "../api";
import { loadSelectedBenchmark, selectBenchmarkConfig } from "../benchmarks";
import { nodeModelByID } from "../data";
import { elementTarget } from "../dom";
import { activateTab } from "../shell/navigation";
import { runTask } from "../tasks";
import { openSelectedWebUI } from "../webuis";
import { editConfigInCook } from "./cook-bindings";

type RuntimeStateRefresher = (nodeID: string) => Promise<void>;

export function bindRuntimeActions(container: HTMLElement, refreshNodeState: RuntimeStateRefresher): void {
  container.addEventListener("click", event => {
    const button = elementTarget(event)?.closest<HTMLButtonElement>("[data-runtime-action]");
    if (!button) {
      return;
    }
    button.closest<HTMLElement>("[popover]")?.hidePopover();
    const {nodeId = "", modelId = "", backendId = "", runtimeId = "", generation = "", webuiId = ""} = button.dataset;
    switch (button.dataset.runtimeAction) {
      case "unload":
        runTask(async () => {
          await unloadNodeRuntime({node_id: nodeId, backend_id: backendId, runtime_id: runtimeId, expected_generation: Number(generation)});
          await refreshNodeState(nodeId);
        }, `runtime-unload-${nodeId}-${runtimeId}`, "nodes", `Unloading ${runtimeId}…`);
        break;
      case "benchmark":
        openBenchmarkFor(nodeId, modelId);
        break;
      case "webui":
        openSelectedWebUI(webuiId);
        break;
      case "edit":
        openConfigEditorFor(nodeId, modelId);
        break;
    }
  });
}

function openBenchmarkFor(nodeID: string, modelID: string): void {
  const model = nodeModelByID(nodeID, modelID);
  if (!model) {
    return;
  }
  selectBenchmarkConfig(model);
  activateTab("benchmarks");
  runTask(loadSelectedBenchmark, "benchmark-load", "benchmark", "Loading benchmark…");
}

function openConfigEditorFor(nodeID: string, modelID: string): void {
  const model = nodeModelByID(nodeID, modelID);
  if (!model) {
    return;
  }
  activateTab("cook");
  runTask(() => editConfigInCook(nodeID, model.local_id), "quick-config-change", "cook-selection", "Opening config…");
}
