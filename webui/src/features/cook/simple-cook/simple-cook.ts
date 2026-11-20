import { errorBody } from "../../../api/client";
import { applyConfigFile, deleteConfigFile, previewConfigFile } from "../../../api/cook-api";
import { exportPortableConfig } from "../../../api/model-assets-api";
import { backendModeKey } from "../../../domain/backend-mode";
import { jinjaKwargsKey, jinjaKwargsPrecedenceKey } from "../../../domain/option-keys";
import { cloneOptions, defaultFieldValue, parseOptionInput } from "../../../domain/option-values";
import { downloadBlob } from "../../../lib/download";
import { setHTML } from "../../../lib/safe-html";
import { optionDefinition } from "../../../state/inventory-selectors";
import { state } from "../../../state/app-state";
import type { ConfigFileRequest, ConfigFileResponse, RefreshInventory } from "../../../types";
import { confirmDestructive } from "../../shell/dialogs";
import { isSimpleCookDirty, markSimpleCookClean } from "../../shell/dirty-state";
import { cookElements } from "../cook-elements";
import { cookResultHTML } from "../cook-result-view";
import { clearConversionScope, confirmPendingConversions, discardConversion, invalidateAcceptedConversions, recordConversion } from "../conversions";
import { defaultConfigForNode } from "./simple-cook-data";
import { loadSimpleConfig, startUnsavedDraft } from "./simple-cook-draft";
import { renderConfigEditor, renderFieldSidebar, renderSimpleCook } from "./simple-cook-render";
import { selectedConfig, selectedNode } from "./simple-cook-selection";
import { importedConfigNameStem, parseImportedOptions } from "./simple-cook-import-data";

type ConfigSubmitter = (request: ConfigFileRequest) => Promise<ConfigFileResponse>;

export async function exportSimpleConfig(): Promise<void> {
  const config = selectedConfig();
  const node = selectedNode();
  if (!config || !node) {
    throw new Error("Select a saved config to export");
  }
  if (isSimpleCookDirty()) {
    throw new Error("Save or discard Simple Cook changes before exporting");
  }
  const content = await exportPortableConfig({node_id: node.node_id, ...(node.node_url ? {node_url: node.node_url} : {}), id: config.local_id, filename: config.filename});
  downloadBlob(content, `${config.local_id}.kcpps`);
}

export async function importSimpleConfig(file: File): Promise<void> {
  startUnsavedDraft("new", parseImportedOptions(await file.text()), importedConfigNameStem(file.name));
  renderSimpleCook();
}

export function selectSimpleNode(nodeID: string): void {
  state.simpleCook.nodeID = nodeID;
  loadSimpleConfig((selectedNode()?.models ?? [])[0] ?? null);
  renderSimpleCook();
}

export function selectSimpleConfig(configID: string): void {
  loadSimpleConfig((selectedNode()?.models ?? []).find(item => item.local_id === configID) ?? null);
  renderSimpleCook();
}

export function updateSimpleFieldFilter(value: string): void {
  state.simpleCook.fieldFilter = value;
  renderConfigEditor();
}

export function updateSimpleSectionOpen(target: EventTarget | null): void {
  if (!(target instanceof HTMLDetailsElement)) {
    return;
  }
  const section = target.dataset.simpleSection;
  if (!section) {
    return;
  }
  const openSections = new Set(state.simpleCook.openSections);
  if (target.open) {
    openSections.add(section);
  } else {
    openSections.delete(section);
  }
  state.simpleCook.openSections = Array.from(openSections);
}

export function newSimpleConfig(): void {
  startUnsavedDraft("new", defaultConfigForNode(selectedNode()), "new-config");
  renderSimpleCook();
}

export function copySimpleConfig(): void {
  startUnsavedDraft("copy", cloneOptions(state.simpleCook.fields), `${selectedConfig()?.local_id || "config"}-copy`);
  renderSimpleCook();
}

export function addSelectedSimpleField(): void {
  const key = cookElements.simpleAddFieldSelect.value;
  if (!key || Object.hasOwn(state.simpleCook.fields, key)) {
    return;
  }
  state.simpleCook.fields[key] = defaultFieldValue(optionDefinition(key));
  invalidateAcceptedConversions();
  renderSimpleCook();
}

export function updateSimpleField(target: EventTarget | null): void {
  if (!(target instanceof HTMLInputElement) && !(target instanceof HTMLSelectElement)) {
    return;
  }
  if (target instanceof HTMLSelectElement && target.dataset.simpleBackendMode !== undefined) {
    state.simpleCook.fields[backendModeKey] = target.value;
    invalidateAcceptedConversions();
    renderSimpleCook();
    return;
  }
  const key = target.dataset.simpleField;
  if (!key) {
    return;
  }
  if (target instanceof HTMLSelectElement && target.multiple) {
    state.simpleCook.fields[key] = [...target.selectedOptions].map(option => option.value);
    discardConversion("quick", key);
    renderSimpleCook();
    return;
  }
  const parsed = parseOptionInput(optionDefinition(key), target.value);
  state.simpleCook.fields[key] = parsed.value;
  recordConversion("quick", key, parsed);
  renderSimpleCook();
}

export function removeSimpleField(key: string): void {
  delete state.simpleCook.fields[key];
  if (key === jinjaKwargsKey) {
    delete state.simpleCook.fields[jinjaKwargsPrecedenceKey];
  }
  discardConversion("quick", key);
  invalidateAcceptedConversions();
  if (state.simpleCook.sidebar?.key === key) {
    state.simpleCook.sidebar = null;
  }
  renderSimpleCook();
}

export function showSimpleFieldValues(key: string, type: "field" | "model"): void {
  state.simpleCook.sidebar = {key, type};
  renderFieldSidebar();
}

export async function previewSimpleCook(): Promise<void> {
  if (!await confirmPendingConversions("quick")) {
    return;
  }
  await submitSimpleConfig(previewConfigFile);
}

export async function applySimpleCook(refreshInventory: RefreshInventory): Promise<void> {
  if (!await confirmPendingConversions("quick")) {
    return;
  }
  const request = simpleConfigRequest();
  if (request.overwrite && !await confirmDestructive("Overwrite config?", `Applying ${request.id || "this config"} replaces the existing configuration.`, "Overwrite")) {
    return;
  }
  const result = await submitSimpleConfig(applyConfigFile);
  if (!result) {
    return;
  }
  state.simpleCook.mode = "edit";
  state.simpleCook.configID = result.id || "";
  state.simpleCook.fields = cloneOptions(result.options ?? state.simpleCook.fields);
  cookElements.cookIdInput.value = result.id || cookElements.cookIdInput.value;
  markSimpleCookClean();
  clearConversionScope("quick");
  await refreshInventory();
}

export async function deleteSimpleConfig(refreshInventory: RefreshInventory): Promise<void> {
  const config = selectedConfig();
  if (!config) {
    return;
  }
  if (!await confirmDestructive("Delete config?", `Delete ${config.filename || config.local_id}? This cannot be undone.`, "Delete")) {
    return;
  }
  try {
    await deleteConfigFile({
      node_id: state.simpleCook.nodeID,
      node_url: selectedNode()?.node_url || "",
      id: config.local_id,
      filename: config.filename,
      overwrite: false,
      options: {}
    });
    await refreshInventory();
  } catch (error) {
    showSimpleCookError(error);
    throw error;
  }
}

async function submitSimpleConfig(submitter: ConfigSubmitter): Promise<ConfigFileResponse | null> {
  try {
    const result = await submitter(simpleConfigRequest());
    setHTML(cookElements.cookOutput, cookResultHTML(result));
    return result;
  } catch (error) {
    showSimpleCookError(error);
    throw error;
  }
}

function simpleConfigRequest(): ConfigFileRequest {
  const config = selectedConfig();
  const requestedID = cookElements.cookIdInput.value.trim();
  return {
    node_id: state.simpleCook.nodeID,
    node_url: selectedNode()?.node_url || "",
    id: requestedID,
    filename: config?.filename || "",
    overwrite: requestedID === config?.local_id || cookElements.overwriteInput.checked,
    options: cloneOptions(state.simpleCook.fields)
  };
}

function showSimpleCookError(error: unknown): void {
  setHTML(cookElements.cookOutput, cookResultHTML(errorBody(error)));
}
