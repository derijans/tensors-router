import { SafeHTML, emptyHTML, html } from "./safe-html";
import type { LendingSettingEntry, LendingSettingSource, Tone } from "./types";
import { badge } from "./markup-primitives";

const sourceTones: Record<LendingSettingSource, Tone> = {default: "neutral", config: "info", db: "warning"};
const sourceLabels: Record<LendingSettingSource, string> = {default: "default", config: "config file", db: "database"};

export interface LendingSettingInput {
  key: string;
  value: string;
}

export interface LendingSettingChanges {
  set: Record<string, string>;
  reset: string[];
}

export function renderLendingSettingRows(entries: LendingSettingEntry[], editable: boolean): SafeHTML {
  return html`${entries.map(entry => renderLendingSettingRow(entry, editable))}`;
}

function renderLendingSettingRow(entry: LendingSettingEntry, editable: boolean): SafeHTML {
  const valueWithoutOverride = entry.config ?? entry.default;
  return html`
    <tr data-lending-setting="${entry.key}">
      <td>
        <strong>${entry.key}</strong>
        <div class="muted lending-setting-description">${entry.description}</div>
      </td>
      <td><code>${entry.default}</code></td>
      <td>${entry.config ? html`<code>${entry.config}</code>` : html`<span class="muted">not set</span>`}</td>
      <td>${entry.kind === "boolean" ? renderBooleanInput(entry, valueWithoutOverride, editable) : renderTextInput(entry, valueWithoutOverride, editable)}</td>
      <td><span class="lending-setting-effective">${badge(entry.effective, sourceTones[entry.source])}<span class="muted">${sourceLabels[entry.source]}</span></span></td>
      <td>${entry.override !== undefined && editable ? html`<button type="button" data-lending-setting-reset="${entry.key}">Default</button>` : emptyHTML}</td>
    </tr>
  `;
}

function renderTextInput(entry: LendingSettingEntry, valueWithoutOverride: string, editable: boolean): SafeHTML {
  return html`<input class="lending-setting-input" type="text" data-lending-setting-input="${entry.key}" value="${entry.override ?? ""}" placeholder="${valueWithoutOverride}" aria-label="Database value for ${entry.key}"${disabledUnless(editable)}>`;
}

function renderBooleanInput(entry: LendingSettingEntry, valueWithoutOverride: string, editable: boolean): SafeHTML {
  const options: [string, string][] = [["", `not set (${valueWithoutOverride})`], ["true", "true"], ["false", "false"]];
  return html`<select class="lending-setting-input" data-lending-setting-input="${entry.key}" aria-label="Database value for ${entry.key}"${disabledUnless(editable)}>
    ${options.map(([value, label]) => html`<option value="${value}"${value === (entry.override ?? "") ? html` selected` : emptyHTML}>${label}</option>`)}
  </select>`;
}

function disabledUnless(editable: boolean): SafeHTML {
  return editable ? emptyHTML : html` disabled`;
}

export function pendingSettingChanges(entries: LendingSettingEntry[], inputs: LendingSettingInput[]): LendingSettingChanges {
  const overrides = new Map(entries.map(entry => [entry.key, entry.override ?? ""]));
  const changes: LendingSettingChanges = {set: {}, reset: []};
  for (const input of inputs) {
    const stored = overrides.get(input.key);
    const value = input.value.trim();
    if (stored === undefined || value === stored) {
      continue;
    }
    if (value === "") {
      changes.reset.push(input.key);
    } else {
      changes.set[input.key] = value;
    }
  }
  return changes;
}
