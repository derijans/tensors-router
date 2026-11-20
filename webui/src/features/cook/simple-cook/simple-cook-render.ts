import { backendModeKey, backendModeOrDefault, isBackendMode } from "../../../domain/backend-mode";
import { jinjaKwargsPrecedenceKey } from "../../../domain/option-keys";
import { optionInputList, optionValueText } from "../../../domain/option-values";
import { unloadPolicyKey } from "../../../domain/unload-policy";
import { html, setHTML } from "../../../lib/safe-html";
import { allOptionDefinitions, optionDefinition } from "../../../state/inventory-selectors";
import { state } from "../../../state/app-state";
import type { FieldRenderContext, JsonValue } from "../../../types";
import { emptyState } from "../../../ui/empty-state";
import { optionElement, replaceOptionsKeepingSelection, selectChoice } from "../../../ui/select";
import { isSimpleCookDirty } from "../../shell/dirty-state";
import { setControlUnavailable } from "../../shell/operations";
import { cookElements } from "../cook-elements";
import { configLabel, nodeLabel, safeID, sectionLabels, sectionModelKeys } from "./simple-cook-data";
import { syncSimpleCookSelection } from "./simple-cook-draft";
import { addableFieldDefinitions, addableFieldLabel, effectiveFieldValue, fieldCountLabel, fieldMatchesFilter, isVirtualField, simpleCookFieldGroups } from "./simple-cook-fields-data";
import { fieldRenderContext, selectedConfig, selectedNode } from "./simple-cook-selection";
import { type SimpleFieldControl, type SimpleFieldRow, configSectionMarkup, fieldRowMarkup, fieldSidebarMarkup } from "./simple-cook-view";
import { comparisonClass, fieldChoices, sidebarValueRows } from "./simple-cook-field-values-data";

export function renderSimpleCook(): void {
  syncSimpleCookSelection();
  renderSimpleSelectors();
  renderAddFieldSelect();
  renderConfigEditor();
  renderFieldSidebar();
}

export function renderConfigEditor(): void {
  const fields = state.simpleCook.fields;
  const backendMode = simpleBackendModeValue();
  const context = fieldRenderContext();
  const openSections = new Set(state.simpleCook.openSections);
  const sections = simpleCookFieldGroups(fields, optionDefinition).flatMap(group => {
    const keys = group.keys.filter(key => fieldMatchesFilter(key, effectiveFieldValue(fields, key, backendMode), state.simpleCook.fieldFilter));
    if (keys.length === 0) {
      return [];
    }
    return [configSectionMarkup({
      section: group.section,
      label: sectionLabels[group.section] || group.section,
      countLabel: fieldCountLabel(keys.length),
      open: openSections.has(group.section),
      rows: keys.map(key => fieldRowMarkup(fieldRow(key, effectiveFieldValue(fields, key, backendMode), group.section, context)))
    })];
  });
  setHTML(cookElements.simpleConfigEditor, sections.length ? html`${sections}` : emptyState("No fields"));
}

export function renderFieldSidebar(): void {
  const sidebar = state.simpleCook.sidebar;
  if (!sidebar) {
    setHTML(cookElements.simpleFieldSidebar, emptyState("Pick Values or Model on a field to compare it with other configs."));
    return;
  }
  setHTML(cookElements.simpleFieldSidebar, fieldSidebarMarkup(sidebar, sidebarValueRows(sidebar.key, sidebar.type, optionDefinition, fieldRenderContext())));
}

function renderSimpleSelectors(): void {
  const nodes = state.inventory?.nodes ?? [];
  replaceOptionsKeepingSelection(cookElements.simpleNodeSelect, nodes.map(node => selectChoice(node.node_id, nodeLabel(node))));
  cookElements.simpleNodeSelect.value = state.simpleCook.nodeID;
  const configs = selectedNode()?.models ?? [];
  replaceOptionsKeepingSelection(cookElements.simpleConfigSelect, configs.map(model => selectChoice(model.local_id, configLabel(model))));
  cookElements.simpleConfigSelect.value = state.simpleCook.configID;
  setControlUnavailable(cookElements.simpleConfigSelect, configs.length === 0);
  setControlUnavailable(cookElements.simpleCopyButton, Object.keys(state.simpleCook.fields).length === 0);
  setControlUnavailable(cookElements.simpleDeleteButton, !selectedConfig());
  setControlUnavailable(cookElements.simpleExportButton, !selectedConfig() || isSimpleCookDirty());
  cookElements.simpleFieldFilter.value = state.simpleCook.fieldFilter;
}

function renderAddFieldSelect(): void {
  const definitions = addableFieldDefinitions(allOptionDefinitions(), state.simpleCook.fields);
  setHTML(cookElements.simpleAddFieldSelect, html`${definitions.map(definition => optionElement(definition.key, addableFieldLabel(definition)))}`);
}

function fieldRow(key: string, value: JsonValue | undefined, section: string, context: FieldRenderContext): SimpleFieldRow {
  const definition = optionDefinition(key);
  return {
    key,
    label: definition?.name || key,
    control: fieldControl(key, value, fieldChoices(key, definition, context)),
    comparisonClass: comparisonClass(key, section, context),
    hasModelValues: Boolean(sectionModelKeys[section]),
    virtual: isVirtualField(state.simpleCook.fields, key)
  };
}

function fieldControl(key: string, value: JsonValue | undefined, choices: string[]): SimpleFieldControl {
  if (key === backendModeKey) {
    return {kind: "backend-mode", value: simpleBackendModeValue()};
  }
  if (key === unloadPolicyKey) {
    return {kind: "unload-policy", values: optionInputList(value)};
  }
  if (key === jinjaKwargsPrecedenceKey) {
    return {kind: "jinja-precedence", value: optionValueText(value)};
  }
  return {kind: "text", value: optionValueText(value), choices, datalistID: `field-values-${safeID(key)}`};
}

function simpleBackendModeValue(): string {
  const value = state.simpleCook.fields[backendModeKey];
  return isBackendMode(value) ? value : backendModeOrDefault(selectedConfig()?.backend_mode || selectedNode()?.backend_mode);
}
