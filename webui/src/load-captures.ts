import { getLoadCaptureDetail, getLoadCaptureOutput, getLoadCaptures } from "./api";
import { elements } from "./elements";
import { state } from "./state";
import { escapeHTML } from "./utils";
import type { LoadCaptureAttempt, LoadCaptureQuery } from "./types";
import { safeTerminalText } from "./terminal-output";

export async function loadLoadCaptures(reset = true): Promise<void> {
  state.loadCaptures.loading = true;
  state.loadCaptures.error = "";
  renderLoadCaptures();
  try {
    const query = captureQuery(reset ? "" : state.loadCaptures.nextCursor);
    const response = await getLoadCaptures(query);
    state.loadCaptures.data = response;
    state.loadCaptures.nextCursor = response.next_cursor || "";
    state.loadCaptures.attempts = reset ? response.attempts : [...state.loadCaptures.attempts, ...response.attempts];
    elements.loadCapturesTab.hidden = !response.enabled;
    elements.loadCapturesPanel.hidden = !response.enabled;
  } catch (error) {
    state.loadCaptures.error = error instanceof Error ? error.message : String(error);
  } finally {
    state.loadCaptures.loading = false;
    renderLoadCaptures();
  }
}

export function updateLoadCaptureFilters(): void {
  state.loadCaptures.query = {
    node_ids: Array.from(elements.loadCaptureNodeSelect.selectedOptions).map(option => option.value),
    status: elements.loadCaptureStatusSelect.value,
    kind: elements.loadCaptureKindSelect.value,
    backend: elements.loadCaptureBackendInput.value.trim(),
    from: dateTimeMillis(elements.loadCaptureFromInput.value),
    to: dateTimeMillis(elements.loadCaptureToInput.value)
  };
}

export async function selectLoadCapture(nodeID: string, attemptID: string): Promise<void> {
  state.loadCaptures.detail = await getLoadCaptureDetail(nodeID, attemptID);
  state.loadCaptures.output = [];
  state.loadCaptures.outputCursor = 0;
  state.loadCaptures.outputMore = state.loadCaptures.detail.attempt.kind === "physical" || Boolean(state.loadCaptures.detail.attempt.physical_attempt_id);
  if (state.loadCaptures.outputMore) {
    await loadMoreCaptureOutput();
  }
  renderLoadCaptureDetail();
}

export async function loadMoreCaptureOutput(): Promise<void> {
  const detail = state.loadCaptures.detail;
  if (!detail || !state.loadCaptures.outputMore) {
    return;
  }
  const page = await getLoadCaptureOutput(detail.attempt.node_id, detail.attempt.id, state.loadCaptures.outputCursor);
  state.loadCaptures.output.push(...page.chunks);
  state.loadCaptures.outputCursor = page.next_sequence || 0;
  state.loadCaptures.outputMore = Boolean(page.next_sequence);
  renderLoadCaptureDetail();
}

export function renderLoadCaptures(): void {
  renderNodeChoices();
  if (state.loadCaptures.error) {
    elements.loadCaptureStatus.innerHTML = `<div class="error-text">${escapeHTML(state.loadCaptures.error)}</div>`;
  } else if (state.loadCaptures.loading) {
    elements.loadCaptureStatus.textContent = "Loading captures...";
  } else {
    elements.loadCaptureStatus.innerHTML = (state.loadCaptures.data?.node_errors || []).map(error => `<div class="error-text">${escapeHTML(error.node_id)}: ${escapeHTML(error.error)}</div>`).join("");
  }
  elements.loadCaptureRows.innerHTML = state.loadCaptures.attempts.map(captureRow).join("");
  elements.loadCaptureMoreButton.hidden = !state.loadCaptures.nextCursor;
  renderLoadCaptureDetail();
}

function renderNodeChoices(): void {
  const nodes = state.loadCaptures.data?.nodes || [];
  const selected = new Set(state.loadCaptures.query.node_ids);
  elements.loadCaptureNodeSelect.innerHTML = nodes.map(node => `<option value="${escapeHTML(node.node_id)}"${selected.has(node.node_id) ? " selected" : ""}>${escapeHTML(node.node_id)}${node.enabled ? "" : " (disabled)"}</option>`).join("");
}

function captureRow(attempt: LoadCaptureAttempt): string {
  const duration = attempt.duration_ms > 0 ? `${attempt.duration_ms} ms` : "N/A";
  return `<tr>
    <td>${escapeHTML(new Date(attempt.started_at).toLocaleString())}</td>
    <td>${escapeHTML(attempt.node_id)}</td>
    <td>${escapeHTML(attempt.kind)}</td>
    <td>${escapeHTML(attempt.status)}</td>
    <td>${escapeHTML(attempt.backend_mode)}</td>
    <td>${escapeHTML(attempt.runtime)} / ${escapeHTML(attempt.lane)}</td>
    <td>${escapeHTML(duration)}</td>
    <td><code>${escapeHTML(attempt.snapshot_sha256.slice(0, 16))}</code><div class="muted">${escapeHTML((attempt.model_hashes || []).map(value => value.slice(0, 24)).join(" "))}</div></td>
    <td>${attempt.truncated ? "Yes" : "No"}</td>
    <td><button type="button" data-load-capture-node="${escapeHTML(attempt.node_id)}" data-load-capture-id="${escapeHTML(attempt.id)}">Inspect</button></td>
  </tr>`;
}

function renderLoadCaptureDetail(): void {
  const detail = state.loadCaptures.detail;
  if (!detail) {
    elements.loadCaptureDetail.innerHTML = "";
    elements.loadCaptureOutput.textContent = "";
    elements.loadCaptureOutputMoreButton.hidden = true;
    return;
  }
  const assets = detail.assets.map(asset => `${asset.role}[${asset.position}] sha256:${asset.sha256}`).join("\n");
  elements.loadCaptureDetail.innerHTML = `<h3>Sanitized KCPPS</h3><pre>${escapeHTML(JSON.stringify(detail.kcpps, null, 2))}</pre><h3>Model hashes</h3><pre>${escapeHTML(assets || "None")}</pre>${detail.attempt.failure_message ? `<h3>Failure</h3><pre>${escapeHTML(detail.attempt.failure_class + ": " + detail.attempt.failure_message)}</pre>` : ""}`;
  elements.loadCaptureOutput.textContent = combinedOutput();
  elements.loadCaptureOutputMoreButton.hidden = !state.loadCaptures.outputMore;
}

function combinedOutput(): string {
  return [...state.loadCaptures.output].sort((left, right) => left.sequence - right.sequence).map(chunk => `[${chunk.stream}] ${safeTerminalText(chunk.payload)}`).join("");
}

function captureQuery(cursor: string): LoadCaptureQuery {
  const query = {...state.loadCaptures.query, cursor};
  if (!query.from) delete query.from;
  if (!query.to) delete query.to;
  return query;
}

function dateTimeMillis(value: string): number | undefined {
  if (!value) return undefined;
  const parsed = new Date(value).getTime();
  return Number.isFinite(parsed) ? parsed : undefined;
}