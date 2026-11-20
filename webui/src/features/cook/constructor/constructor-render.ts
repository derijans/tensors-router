import { backendModeKey, backendModeLabels, backendModeOrDefault, backendModes, isBackendMode } from "../../../domain/backend-mode";
import { laneKinds } from "../../../domain/lanes";
import { compareOptionKeys } from "../../../domain/option-keys";
import { html, type SafeHTML, setHTML } from "../../../lib/safe-html";
import { matchesQuery } from "../../../lib/text";
import { state } from "../../../state/app-state";
import { nodeByID } from "../../../state/inventory-selectors";
import type { LaneKind, NodeInventory, PaletteComponentPayload, PaletteEntry } from "../../../types";
import { emptyState, listOrFallback } from "../../../ui/empty-state";
import { renderIssue } from "../../../ui/issue";
import { optionElement } from "../../../ui/select";
import { cookElements } from "../cook-elements";
import { renderFieldEditor } from "./constructor-field-editor";
import { configPaletteEntries, filePaletteEntries, optionPaletteEntries } from "./constructor-palette";
import { currentValidationIssues } from "./constructor-current-request";
import { selectedOptionsForInspector, usedPaths } from "./constructor-selection-data";
import { emptyLaneMarkup, inheritedOptionRow, laneShell, limitedRows, optionEditorRow, paletteItemMarkup, selectedLaneMarkup, usedModelRows } from "./constructor-view";

export function renderConstructor(): void {
  renderBackendSelector();
  renderPalette();
  renderLanes();
  renderInspector();
  renderFieldEditor();
}

export function renderInspector(): void {
  renderValidation();
  const used = laneKinds.flatMap(lane => {
    const selected = state.constructor.lanes[lane];
    return selected ? usedModelRows(lane, selected, usedPaths(selected)) : [];
  });
  setHTML(cookElements.usedModelsList, used.length ? html`${limitedRows(used, state.constructor.showUsedAll, "used")}` : emptyState("No models selected"));
  const options = selectedOptionRows();
  setHTML(cookElements.selectedOptionsList, options.length ? html`${limitedRows(options, state.constructor.showOptionsAll, "options")}` : emptyState("No options selected"));
}

export function renderValidation(): void {
  const validation = currentValidationIssues();
  setHTML(cookElements.validationList, validation.length ? html`${validation.map(renderIssue)}` : emptyState("Clean"));
}

function renderBackendSelector(): void {
  const value = constructorBackendMode();
  setHTML(cookElements.advancedBackendSelect, html`${backendModes.map(mode => optionElement(mode, backendModeLabels[mode], mode === value))}`);
  cookElements.advancedBackendSelect.classList.toggle("virtual-backend-select", !state.constructor.backendTouched);
}

function renderPalette(): void {
  const query = cookElements.constructorFilterInput.value;
  const entries = paletteEntries().filter(entry => matchesQuery(JSON.stringify(entry), query));
  state.palettePayloads = {};
  setHTML(cookElements.paletteList, listOrFallback(entries.map(registerPaletteEntry), emptyState("No items")));
}

function registerPaletteEntry(entry: PaletteEntry): SafeHTML {
  const payloadID = `payload-${Object.keys(state.palettePayloads).length}`;
  state.palettePayloads[payloadID] = entry.payload;
  return paletteItemMarkup(entry, payloadID);
}

function renderLanes(): void {
  setHTML(cookElements.constructorLanes, html`${laneKinds.map(laneShell)}`);
  const nodes = state.inventory?.nodes ?? [];
  for (const lane of laneKinds) {
    const drop = document.querySelector(`[data-drop-lane="${lane}"]`);
    if (!(drop instanceof HTMLElement)) {
      continue;
    }
    const selected = state.constructor.lanes[lane];
    setHTML(drop, selected ? selectedLaneMarkup({
      lane,
      selected,
      nodes,
      targetNodeID: ensureLaneTargetNode(lane, selected, nodes),
      overrideCount: Object.keys(state.constructor.laneOptions[lane] ?? {}).length
    }) : emptyLaneMarkup(lane));
  }
}

function ensureLaneTargetNode(lane: LaneKind, selected: PaletteComponentPayload, nodes: readonly NodeInventory[]): string {
  const current = state.constructor.targetNodes[lane] || selected.component.node_id || nodes[0]?.node_id || "";
  if (!state.constructor.targetNodes[lane]) {
    state.constructor.targetNodes[lane] = current;
  }
  return current;
}

function selectedOptionRows(): SafeHTML[] {
  return Object.entries(selectedOptionsForInspector(state.constructor))
    .sort(([left], [right]) => compareOptionKeys(left, right))
    .map(([key, value]) => Object.hasOwn(state.constructor.options, key)
      ? optionEditorRow(key, state.constructor.options[key])
      : inheritedOptionRow(key, value, laneOverrideForKey(key)));
}

function laneOverrideForKey(key: string): LaneKind | null {
  return laneKinds.find(lane => Object.hasOwn(state.constructor.laneOptions[lane] ?? {}, key)) ?? null;
}

function paletteEntries(): PaletteEntry[] {
  if (state.activePalette === "files") {
    return filePaletteEntries();
  }
  if (state.activePalette === "options") {
    return optionPaletteEntries();
  }
  return configPaletteEntries();
}

function constructorBackendMode(): string {
  if (state.constructor.backendTouched && isBackendMode(state.constructor.backendMode)) {
    return state.constructor.backendMode;
  }
  const lanes = laneKinds.flatMap(lane => {
    const selected = state.constructor.lanes[lane];
    return selected ? [{lane, selected}] : [];
  });
  const sourceBackend = lanes.map(({selected}) => selected.model?.options?.[backendModeKey]).find(isBackendMode);
  if (sourceBackend) {
    return sourceBackend;
  }
  const nodeBackend = lanes
    .map(({lane, selected}) => nodeByID(state.constructor.targetNodes[lane] || selected.component.node_id || "")?.backend_mode)
    .find(isBackendMode);
  return nodeBackend ?? backendModeOrDefault(state.inventory?.nodes?.[0]?.backend_mode);
}
