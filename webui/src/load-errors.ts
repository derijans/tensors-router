import { SafeHTML, emptyHTML, html, setHTML } from "./safe-html";
import { getLoadErrors } from "./api";
import { elements } from "./elements";
import { state } from "./state";
import { stripTerminalControls } from "./terminal-output";
import { reportErrorToConsole } from "./console-report";
import type { LoadErrorRecord } from "./types";

const phaseOptions = [
  "config_parse", "asset_resolve", "port_bind", "process_spawn", "health_wait",
  "readiness_watch", "capture_write", "preload", "unload", "separate_runtime", "startup", "download", "other"
];

export async function loadLoadErrors(): Promise<void> {
  ensurePhaseOptions();
  state.loadErrors.loading = true;
  state.loadErrors.error = "";
  renderLoadErrors();
  try {
    const response = await getLoadErrors({
      phase: elements.loadErrorPhaseSelect.value,
      severity: elements.loadErrorSeveritySelect.value
    });
    state.loadErrors.records = response.records;
    state.loadErrors.enabled = response.enabled;
    state.loadErrors.nodeErrors = response.node_errors || [];
    elements.loadErrorsTab.hidden = !response.enabled;
    elements.loadErrorsPanel.hidden = !response.enabled;
    if (!response.records.some(record => record.id === state.loadErrors.selectedID)) {
      state.loadErrors.selectedID = "";
    }
  } catch (error) {
    state.loadErrors.error = error instanceof Error ? error.message : String(error);
  } finally {
    state.loadErrors.loading = false;
    renderLoadErrors();
  }
}

export function selectLoadError(id: string): void {
  state.loadErrors.selectedID = state.loadErrors.selectedID === id ? "" : id;
  renderLoadErrors();
}

function ensurePhaseOptions(): void {
  if (elements.loadErrorPhaseSelect.options.length > 1) {
    return;
  }
  for (const phase of phaseOptions) {
    const option = document.createElement("option");
    option.value = phase;
    option.textContent = phase.replace(/_/g, " ");
    elements.loadErrorPhaseSelect.append(option);
  }
}

function renderLoadErrors(): void {
  const parts: SafeHTML[] = [];
  if (state.loadErrors.loading) {
    parts.push(html`<p class="action-status">Loading…</p>`);
  }
  if (state.loadErrors.error) {
    parts.push(html`<p class="error-text">${state.loadErrors.error}</p>`);
    reportErrorToConsole("load-errors panel", new Error(state.loadErrors.error));
  }
  for (const nodeError of state.loadErrors.nodeErrors) {
    parts.push(html`<p class="error-text">${nodeError.node_id}: ${nodeError.error}</p>`);
  }
  if (!state.loadErrors.loading && state.loadErrors.records.length === 0 && !state.loadErrors.error) {
    parts.push(html`<p class="muted">No pre-load errors recorded.</p>`);
  }
  setHTML(elements.loadErrorStatus, html`${parts}`);

  setHTML(elements.loadErrorRows, html`${state.loadErrors.records.map(record => {
    const selected = record.id === state.loadErrors.selectedID ? html` class="selected"` : emptyHTML;
    return html`<tr data-load-error-id="${record.id}"${selected}>
      <td>${formatTimestamp(record.last_seen_at)}</td>
      <td>${record.node_id || ""}</td>
      <td>${record.phase}</td>
      <td>${record.severity}</td>
      <td>${record.source || ""}</td>
      <td>${record.occurrences}</td>
      <td>${truncate(record.message, 140)}</td>
    </tr>`;
  })}`);

  const detail = state.loadErrors.records.find(record => record.id === state.loadErrors.selectedID);
  setHTML(elements.loadErrorDetail, detail ? renderDetail(detail) : emptyHTML);
  elements.loadErrorOutput.textContent = detail?.output ? stripTerminalControls(detail.output) : "";
}

function renderDetail(record: LoadErrorRecord): SafeHTML {
  const rows: [string, string][] = [
    ["Phase", record.phase],
    ["Severity", record.severity],
    ["Source", record.source || ""],
    ["Node", record.node_id || ""],
    ["Config", record.config_name || ""],
    ["Backend", [record.backend, record.backend_mode].filter(Boolean).join(" / ")],
    ["Model", record.model_id || ""],
    ["First seen", formatTimestamp(record.first_seen_at)],
    ["Last seen", formatTimestamp(record.last_seen_at)],
    ["Occurrences", String(record.occurrences)],
    ["Exit", record.exit_error || ""],
    ["Truncated", record.truncated ? "yes" : "no"]
  ];
  return html`
    <h3>${record.message}</h3>
    <dl class="load-error-detail">
      ${rows.filter(([, value]) => value !== "").map(([label, value]) => html`<div><dt>${label}</dt><dd>${value}</dd></div>`)}
    </dl>
  `;
}

function formatTimestamp(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
}

function truncate(value: string, limit: number): string {
  return value.length > limit ? `${value.slice(0, limit - 1)}…` : value;
}
