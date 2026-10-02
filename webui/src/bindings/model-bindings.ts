import { updateModelState } from "../api";
import { refreshInventory } from "../app-data";
import { elementTarget } from "../dom";
import { elements } from "../elements";
import { loadSelectedConfig, resolveFilteredModels, retryModelAssetResolution } from "../model-actions";
import { calculateModelFileHash, copyModelFileHash } from "../model-files";
import { activateModelInventorySubtab, updateConfigNodeFilter, updateFileNodeFilter } from "../model-inventory";
import { persistModelEnabled } from "../model-state-action";
import { hasRoomForEveryColumn, isModelColumnKey, type ModelColumnChoices } from "../model-columns";
import { loadModelColumnChoices, saveModelColumnChoices } from "../model-column-preferences";
import { renderTables } from "../render-dashboard";
import { openRoutingLinksDialog } from "../routing-links-dialog";
import { openSeparateRuntimeDialog } from "../separate-runtime-dialog";
import { onTabActivation } from "../shell/navigation";
import { state } from "../state";
import { runTask } from "../tasks";

export function bindModels(): void {
  onTabActivation(tab => {
    if (tab === "models") {
      runTask(() => refreshInventory(true), "models-inventory", "models", "Scanning model files…");
    }
  });
  elements.modelInventorySubtabs.addEventListener("click", event => {
    const subtab = elementTarget(event)?.dataset.modelInventorySubtab;
    if (subtab === "models" || subtab === "files") {
      activateModelInventorySubtab(subtab);
    }
  });
  bindInventoryFilters();
  bindModelColumns();
  elements.resolveFilteredModelsButton.addEventListener("click", () => runTask(() => resolveFilteredModels(refreshInventory), "resolve-filtered-models", "models", "Resolving visible configs…"));
  elements.modelsActionStatus.addEventListener("click", event => {
    const key = elementTarget(event)?.dataset.modelResolutionRetry;
    if (key) {
      runTask(() => retryModelAssetResolution(key, refreshInventory), `resolve-retry-${key}`, "models", "Retrying resolution…");
    }
  });
  elements.modelsTable.addEventListener("click", handleModelRowClick);
  elements.modelsTable.addEventListener("change", handleModelEnabledChange);
  elements.filesTable.addEventListener("click", event => {
    const target = elementTarget(event);
    const nodeID = target?.dataset.hashFileNode;
    const path = target?.dataset.hashFilePath;
    if (nodeID && path) {
      runTask(() => calculateModelFileHash(nodeID, path), `hash-file-${encodeURIComponent(nodeID)}-${encodeURIComponent(path)}`, "models", "Hashing model file…");
      return;
    }
    const hash = target?.dataset.copyFileHash;
    if (hash) {
      runTask(() => copyModelFileHash(hash), `copy-file-hash-${hash}`, "models-copy", "Copying SHA-256…");
    }
  });
}

export function applyModelSearch(query: string): void {
  state.models.modelSearch = query;
  elements.modelSearchInput.value = query;
  activateModelInventorySubtab("models");
  renderTables();
}

function bindModelColumns(): void {
  state.models.columnChoices = loadModelColumnChoices();
  elements.modelColumnsList.addEventListener("change", event => {
    const target = elementTarget(event);
    const column = target?.dataset.modelColumn;
    if (target instanceof HTMLInputElement && isModelColumnKey(column)) {
      applyColumnChoices({...state.models.columnChoices, [column]: target.checked});
    }
  });
  elements.modelColumnsResetButton.addEventListener("click", () => applyColumnChoices({}));
  new ResizeObserver(entries => {
    const width = entries.at(-1)?.contentRect.width ?? 0;
    const roomForEveryColumn = hasRoomForEveryColumn(width);
    if (width > 0 && roomForEveryColumn !== state.models.roomForEveryColumn) {
      state.models.roomForEveryColumn = roomForEveryColumn;
      renderTables();
    }
  }).observe(elements.modelsTableCard);
}

function applyColumnChoices(choices: ModelColumnChoices): void {
  state.models.columnChoices = choices;
  saveModelColumnChoices(choices);
  renderTables();
}

function bindInventoryFilters(): void {
  const textFilters: [HTMLInputElement, (value: string) => void][] = [
    [elements.modelSearchInput, value => { state.models.modelSearch = value; }],
    [elements.fileSearchInput, value => { state.models.fileSearch = value; }]
  ];
  for (const [input, apply] of textFilters) {
    input.addEventListener("input", () => {
      apply(input.value);
      renderTables();
    });
  }
  const selectFilters: [HTMLSelectElement, (value: string) => void][] = [
    [elements.modelEnabledFilter, value => { state.models.enabledFilter = value; }],
    [elements.modelBackendFilter, value => { state.models.backendFilter = value; }],
    [elements.modelCapabilityFilter, value => { state.models.capabilityFilter = value; }],
    [elements.fileRoleFilter, value => { state.models.fileRoleFilter = value; }],
    [elements.fileExtensionFilter, value => { state.models.fileExtensionFilter = value; }],
    [elements.fileHashFilter, value => { state.models.fileHashFilter = value; }]
  ];
  for (const [select, apply] of selectFilters) {
    select.addEventListener("change", () => {
      apply(select.value);
      renderTables();
    });
  }
  elements.modelsNodeFilter.addEventListener("change", () => updateConfigNodeFilter(selectedValues(elements.modelsNodeFilter)));
  elements.filesNodeFilter.addEventListener("change", () => updateFileNodeFilter(selectedValues(elements.filesNodeFilter)));
}

function selectedValues(select: HTMLSelectElement): string[] {
  return [...select.selectedOptions].map(option => option.value);
}

function handleModelRowClick(event: Event): void {
  const target = elementTarget(event);
  const modelID = target?.dataset.loadConfig;
  if (modelID) {
    runTask(() => loadSelectedConfig(modelID, refreshInventory), `model-load-${modelID}`, "webui", "Loading model…");
    return;
  }
  const routingLane = target?.dataset.routingLane;
  const routingNode = target?.dataset.routingNode;
  const routingModel = target?.dataset.routingModel;
  if ((routingLane === "image" || routingLane === "text") && routingNode && routingModel) {
    runTask(
      () => openRoutingLinksDialog(routingLane, {node_id: routingNode, model_id: routingModel}),
      `routing-links-${routingLane}-${routingNode}-${routingModel}`,
      "models",
      "Loading routing links"
    );
    return;
  }
  const separateNode = target?.dataset.separateNode;
  const separateId = target?.dataset.separateId;
  if (separateNode && separateId) {
    runTask(
      () => openSeparateRuntimeDialog(separateNode, separateId),
      `separate-runtime-${separateNode}-${separateId}`,
      "models",
      "Loading separate runtime settings"
    );
  }
}

function handleModelEnabledChange(event: Event): void {
  const target = elementTarget(event);
  if (!(target instanceof HTMLInputElement)) {
    return;
  }
  const nodeID = target.dataset.modelEnabledNode;
  const localID = target.dataset.modelEnabledId;
  if (!nodeID || !localID) {
    return;
  }
  const enabled = target.checked;
  runTask(async () => {
    await persistModelEnabled(
      {node_id: nodeID, local_id: localID, enabled},
      updateModelState,
      () => refreshInventory(true),
      () => { target.checked = !enabled; }
    );
  }, `model-state-${nodeID}-${localID}`, `model-state-${nodeID}-${localID}`, `${enabled ? "Enabling" : "Disabling"} ${localID}…`);
}
