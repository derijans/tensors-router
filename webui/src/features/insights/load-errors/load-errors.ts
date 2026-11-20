import { errorMessage } from "../../../lib/errors";
import { SafeHTML, emptyHTML, html, setHTML } from "../../../lib/safe-html";
import { clearLoadErrors, getLoadErrors } from "../../../api/insights-api";
import { confirmDestructive } from "../../shell/dialogs";
import { state } from "../../../state/app-state";
import { stripTerminalControls } from "../../../lib/terminal-output";
import { reportErrorToConsole } from "../../../api/console-report";
import type { LoadErrorRecord } from "../../../types";
import { badge } from "../../../ui/badge";
import { formatTimestamp } from "../../../lib/format-time";
import { truncate } from "../../../lib/text";
import { presentFactGrid } from "../../../ui/fact-grid";
import { actionStatus, errorText, mutedText, nodeErrorText } from "../../../ui/text";
import { insightsElements } from "../insights-elements";

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
      phase: insightsElements.loadErrorPhaseSelect.value,
      severity: insightsElements.loadErrorSeveritySelect.value
    });
    state.loadErrors.records = response.records;
    state.loadErrors.enabled = response.enabled;
    state.loadErrors.nodeErrors = response.node_errors || [];
    insightsElements.loadErrorsTab.hidden = !response.enabled;
    insightsElements.loadErrorsPanel.hidden = !response.enabled;
    if (!response.records.some(record => record.id === state.loadErrors.selectedID)) {
      state.loadErrors.selectedID = "";
    }
  } catch (error) {
    state.loadErrors.error = errorMessage(error);
  } finally {
    state.loadErrors.loading = false;
    renderLoadErrors();
  }
}

export async function clearAllLoadErrors(): Promise<void> {
  const confirmed = await confirmDestructive(
    "Clear all errors",
    "Every recorded error on every node is deleted. Nothing else keeps a copy.",
    "Clear all"
  );
  if (!confirmed) {
    return;
  }
  const response = await clearLoadErrors();
  const failures = response.node_errors ?? [];
  await loadLoadErrors();
  if (failures.length > 0) {
    throw new Error(failures.map(failure => `${failure.node_id}: ${failure.error}`).join("; "));
  }
}

export function selectLoadError(id: string): void {
  state.loadErrors.selectedID = state.loadErrors.selectedID === id ? "" : id;
  renderLoadErrors();
}

function ensurePhaseOptions(): void {
  if (insightsElements.loadErrorPhaseSelect.options.length > 1) {
    return;
  }
  for (const phase of phaseOptions) {
    const option = document.createElement("option");
    option.value = phase;
    option.textContent = phase.replaceAll("_", " ");
    insightsElements.loadErrorPhaseSelect.append(option);
  }
}

function renderLoadErrors(): void {
  insightsElements.errorsFlag.hidden = !state.loadErrors.records.some(record => record.severity === "error");
  const parts: SafeHTML[] = [];
  if (state.loadErrors.loading) {
    parts.push(actionStatus("Loading…"));
  }
  if (state.loadErrors.error) {
    parts.push(errorText(state.loadErrors.error));
    reportErrorToConsole("load-errors panel", new Error(state.loadErrors.error));
  }
  for (const nodeError of state.loadErrors.nodeErrors) {
    parts.push(nodeErrorText(nodeError));
  }
  if (!state.loadErrors.loading && state.loadErrors.records.length === 0 && !state.loadErrors.error) {
    parts.push(mutedText("No pre-load errors recorded.", "p"));
  }
  setHTML(insightsElements.loadErrorStatus, html`${parts}`);

  setHTML(insightsElements.loadErrorRows, html`${state.loadErrors.records.map(record => {
    const selected = record.id === state.loadErrors.selectedID ? html` class="selected"` : emptyHTML;
    return html`<tr data-load-error-id="${record.id}"${selected}>
      <td>${formatTimestamp(record.last_seen_at)}</td>
      <td>${record.node_id || ""}</td>
      <td>${record.phase}</td>
      <td>${badge(record.severity, record.severity === "error" ? "danger" : "warning")}</td>
      <td>${record.source || ""}</td>
      <td>${record.occurrences}</td>
      <td>${truncate(record.message, 140)}</td>
    </tr>`;
  })}`);

  const detail = state.loadErrors.records.find(record => record.id === state.loadErrors.selectedID);
  setHTML(insightsElements.loadErrorDetail, detail ? renderDetail(detail) : emptyHTML);
  insightsElements.loadErrorOutput.textContent = detail?.output ? stripTerminalControls(detail.output) : "";
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
    ${presentFactGrid(rows)}
  `;
}
