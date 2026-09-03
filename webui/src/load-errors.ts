import { getLoadErrors } from "./api";
import { elements } from "./elements";
import { state } from "./state";
import { escapeAttribute, escapeHTML } from "./utils";
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
  const parts: string[] = [];
  if (state.loadErrors.loading) {
    parts.push(`<p class="action-status">Loading…</p>`);
  }
  if (state.loadErrors.error) {
    parts.push(`<p class="error-text">${escapeHTML(state.loadErrors.error)}</p>`);
    reportErrorToConsole("load-errors panel", new Error(state.loadErrors.error));
  }
  for (const nodeError of state.loadErrors.nodeErrors) {
    parts.push(`<p class="error-text">${escapeHTML(nodeError.node_id)}: ${escapeHTML(nodeError.error)}</p>`);
  }
  if (!state.loadErrors.loading && state.loadErrors.records.length === 0 && !state.loadErrors.error) {
    parts.push(`<p class="muted">No pre-load errors recorded.</p>`);
  }
  elements.loadErrorStatus.innerHTML = parts.join("");

  elements.loadErrorRows.innerHTML = state.loadErrors.records.map(record => {
    const selected = record.id === state.loadErrors.selectedID ? " class=\"selected\"" : "";
    return `<tr data-load-error-id="${escapeAttribute(record.id)}"${selected}>
      <td>${escapeHTML(formatTimestamp(record.last_seen_at))}</td>
      <td>${escapeHTML(record.node_id || "")}</td>
      <td>${escapeHTML(record.phase)}</td>
      <td>${escapeHTML(record.severity)}</td>
      <td>${escapeHTML(record.source || "")}</td>
      <td>${record.occurrences}</td>
      <td>${escapeHTML(truncate(record.message, 140))}</td>
    </tr>`;
  }).join("");

  const detail = state.loadErrors.records.find(record => record.id === state.loadErrors.selectedID);
  elements.loadErrorDetail.innerHTML = detail ? renderDetail(detail) : "";
  elements.loadErrorOutput.textContent = detail?.output ? stripTerminalControls(detail.output) : "";
}

function renderDetail(record: LoadErrorRecord): string {
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
  return `
    <h3>${escapeHTML(record.message)}</h3>
    <dl class="load-error-detail">
      ${rows.filter(([, value]) => value !== "").map(([label, value]) => `<div><dt>${escapeHTML(label)}</dt><dd>${escapeHTML(value)}</dd></div>`).join("")}
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
