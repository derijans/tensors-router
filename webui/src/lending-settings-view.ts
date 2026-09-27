import { SafeHTML, emptyHTML, html } from "./safe-html";
import type { LendingSettingEntry, LendingSettingSource } from "./types";
import { chip } from "./utils";

const sourceColors: Record<LendingSettingSource, string> = {default: "violet", config: "cyan", db: "amber"};
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
      <td><input class="lending-setting-input" type="text" data-lending-setting-input="${entry.key}" value="${entry.override ?? ""}" placeholder="${valueWithoutOverride}" aria-label="Database value for ${entry.key}"${editable ? emptyHTML : html` disabled`}></td>
      <td class="lending-setting-effective">${chip(entry.effective, sourceColors[entry.source])}<span class="muted">${sourceLabels[entry.source]}</span></td>
      <td>${entry.override !== undefined && editable ? html`<button type="button" data-lending-setting-reset="${entry.key}">Default</button>` : emptyHTML}</td>
    </tr>
  `;
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
