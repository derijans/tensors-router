import { backendModeLabels, backendModeOrDefault, backendModes } from "../../../domain/backend-mode";
import { jinjaKwargsPrecedenceKey } from "../../../domain/option-keys";
import { unloadPolicies, unloadPolicyKey, unloadPolicyLabels } from "../../../domain/unload-policy";
import { type SafeHTML, html } from "../../../lib/safe-html";
import { attributeFlag } from "../../../ui/attributes";
import { closeButton } from "../../../ui/buttons";
import { emptyState } from "../../../ui/empty-state";
import { optionElement } from "../../../ui/select";
import { mutedText } from "../../../ui/text";
import type { SidebarValueRow, SimpleCookSidebar } from "../../../types";
import { jinjaKwargsPrecedenceOptions } from "../jinja-precedence-options-view";

export type SimpleFieldControl =
  | {kind: "backend-mode"; value: string}
  | {kind: "unload-policy"; values: string[]}
  | {kind: "jinja-precedence"; value: string}
  | {kind: "text"; value: string; choices: string[]; datalistID: string};

export interface SimpleFieldRow {
  key: string;
  label: string;
  control: SimpleFieldControl;
  comparisonClass: string;
  hasModelValues: boolean;
  virtual: boolean;
}

export interface ConfigSectionView {
  section: string;
  label: string;
  countLabel: string;
  open: boolean;
  rows: SafeHTML[];
}

export function configSectionMarkup(view: ConfigSectionView): SafeHTML {
  return html`
    <details class="config-section" data-simple-section="${view.section}"${attributeFlag("open", view.open)}>
      <summary>
        <span>${view.label}</span>
        <span class="section-count">${view.countLabel}</span>
      </summary>
      <div class="config-fields">${view.rows}</div>
    </details>
  `;
}

export function fieldRowMarkup(row: SimpleFieldRow): SafeHTML {
  const modelButton = row.hasModelValues
    ? html`<button class="field-tool" type="button" title="Values used with the same model file" data-field-model-values="${row.key}">Model</button>`
    : "";
  return html`
    <div class="config-field ${row.comparisonClass}${row.virtual ? " backend-virtual" : ""}">
      <div class="field-label">
        <span>${row.label}</span>
        <code>${row.key}</code>
      </div>
      <div class="field-control">
        ${fieldControlMarkup(row.key, row.control, row.virtual)}
      </div>
      <div class="field-buttons">
        <button class="field-tool" type="button" title="Values other configs use" data-field-values="${row.key}">Values</button>
        ${modelButton}
        ${row.virtual ? "" : closeButton(`Remove ${row.key}`, {title: "Remove field", "data-remove-simple-field": row.key})}
      </div>
    </div>
  `;
}

export function fieldSidebarMarkup(sidebar: SimpleCookSidebar, rows: readonly SidebarValueRow[]): SafeHTML {
  return html`
    <div class="field-sidebar-head">
      <div>
        <h3>${sidebar.key}</h3>
        ${mutedText(sidebar.type === "model" ? "same model file" : "same field", "p")}
      </div>
      ${closeButton("Close field values", {"data-close-field-sidebar": true})}
    </div>
    <div class="detail-list">
      ${rows.length ? html`${rows.map(sidebarValueRowMarkup)}` : emptyState("No values")}
    </div>
  `;
}

function fieldControlMarkup(key: string, control: SimpleFieldControl, virtual: boolean): SafeHTML {
  switch (control.kind) {
    case "backend-mode":
      return backendModeSelect(control.value, virtual);
    case "unload-policy":
      return unloadPolicySelect(control.values, virtual);
    case "jinja-precedence":
      return jinjaKwargsPrecedenceSelect(control.value, virtual);
    case "text":
      return html`
        <input data-simple-field="${key}" list="${control.datalistID}" value="${control.value}">
        <datalist id="${control.datalistID}">
          ${control.choices.map(choice => html`<option value="${choice}"></option>`)}
        </datalist>
      `;
  }
}

function backendModeSelect(value: string, virtual: boolean): SafeHTML {
  const selectedValue = backendModeOrDefault(value);
  return html`
    <select data-simple-backend-mode class="${virtual ? "virtual-backend-select virtual-runtime-select" : ""}">
      ${backendModes.map(mode => optionElement(mode, backendModeLabels[mode], mode === selectedValue))}
    </select>
  `;
}

function unloadPolicySelect(values: string[], virtual: boolean): SafeHTML {
  const selected = new Set(values);
  const known = new Set<string>(unloadPolicies);
  const customOptions = html`${values
    .filter(value => !known.has(value))
    .map(value => optionElement(value, value, true))}`;
  return html`
    <select multiple data-simple-field="${unloadPolicyKey}" class="${virtual ? "virtual-runtime-select" : ""}">
      ${customOptions}
      ${unloadPolicies.map(policy => optionElement(policy, unloadPolicyLabels[policy], selected.has(policy)))}
    </select>
  `;
}

function jinjaKwargsPrecedenceSelect(value: string, virtual: boolean): SafeHTML {
  const selectedValue = value === "client" ? "client" : "config";
  return html`
    <select data-simple-field="${jinjaKwargsPrecedenceKey}" class="${virtual ? "virtual-runtime-select" : ""}">
      ${jinjaKwargsPrecedenceOptions(selectedValue)}
    </select>
  `;
}

function sidebarValueRowMarkup(row: SidebarValueRow): SafeHTML {
  return html`
    <div class="sidebar-value">
      <strong>${row.value}</strong>
      ${mutedText(row.config)}
    </div>
  `;
}
