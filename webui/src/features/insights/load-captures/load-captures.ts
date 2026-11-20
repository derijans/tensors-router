import { SafeHTML, emptyHTML, html, setHTML } from "../../../lib/safe-html";
import { getLoadCaptureDetail, getLoadCaptureOutput, getLoadCaptures } from "../../../api/insights-api";
import { state } from "../../../state/app-state";
import type { LoadCaptureAttempt, LoadCaptureQuery, Tone } from "../../../types";
import { badge } from "../../../ui/badge";
import { safeTerminalText } from "../../../lib/terminal-output";
import { errorMessage } from "../../../lib/errors";
import { timestampMillis } from "../../../lib/format-time";
import { optionElement } from "../../../ui/select";
import { errorText, mutedText, nodeErrorText } from "../../../ui/text";
import { insightsElements } from "../insights-elements";
import { lookupOr } from "../../../lib/collections";

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
    insightsElements.loadCapturesTab.hidden = !response.enabled;
    insightsElements.loadCapturesPanel.hidden = !response.enabled;
  } catch (error) {
    state.loadCaptures.error = errorMessage(error);
  } finally {
    state.loadCaptures.loading = false;
    renderLoadCaptures();
  }
}

export function updateLoadCaptureFilters(): void {
  state.loadCaptures.query = {
    node_ids: Array.from(insightsElements.loadCaptureNodeSelect.selectedOptions).map(option => option.value),
    status: insightsElements.loadCaptureStatusSelect.value,
    kind: insightsElements.loadCaptureKindSelect.value,
    backend: insightsElements.loadCaptureBackendInput.value.trim(),
    from: timestampMillis(insightsElements.loadCaptureFromInput.value),
    to: timestampMillis(insightsElements.loadCaptureToInput.value)
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
    setHTML(insightsElements.loadCaptureStatus, errorText(state.loadCaptures.error, "div"));
  } else if (state.loadCaptures.loading) {
    insightsElements.loadCaptureStatus.textContent = "Loading captures...";
  } else {
    setHTML(insightsElements.loadCaptureStatus, html`${(state.loadCaptures.data?.node_errors || []).map(captureNodeError)}`);
  }
  setHTML(insightsElements.loadCaptureRows, html`${state.loadCaptures.attempts.map(captureRow)}`);
  insightsElements.loadCaptureMoreButton.hidden = !state.loadCaptures.nextCursor;
  renderLoadCaptureDetail();
}

function renderNodeChoices(): void {
  const nodes = state.loadCaptures.data?.nodes || [];
  const selected = new Set(state.loadCaptures.query.node_ids);
  setHTML(insightsElements.loadCaptureNodeSelect, html`${nodes.map(node => optionElement(node.node_id, captureNodeLabel(node), selected.has(node.node_id)))}`);

}

function captureNodeLabel(node: {node_id: string; enabled: boolean}): string {
  return node.enabled ? node.node_id : `${node.node_id} (disabled)`;
}

function captureNodeError(
error: {node_id: string; error: string}): SafeHTML {
  return nodeErrorText(error, "div");
}

function captureRow(
attempt: LoadCaptureAttempt): SafeHTML {
  const duration = attempt.duration_ms > 0 ? `${attempt.duration_ms} ms` : "N/A";
  const selected = state.loadCaptures.detail?.attempt.id === attempt.id ? html` class="selected"` : emptyHTML;
  return html`<tr${selected}>
    <td>${new Date(attempt.started_at).toLocaleString()}</td>
    <td>${attempt.node_id}</td>
    <td>${badge(attempt.kind, "neutral")}</td>
    <td>${badge(attempt.status, captureStatusTone(attempt.status))}</td>
    <td>${attempt.backend_mode}</td>
    <td>${attempt.runtime} / ${attempt.lane}</td>
    <td>${duration}</td>
    <td><code>${attempt.snapshot_sha256.slice(0, 16)}</code>${mutedText((attempt.model_hashes || []).map(value => value.slice(0, 24)).join(" "), "div")}</td>
    <td>${attempt.truncated ? "Yes" : "No"}</td>
    <td><button type="button" data-load-capture-node="${attempt.node_id}" data-load-capture-id="${attempt.id}">Inspect</button></td>
  </tr>`;
}

const captureStatusTones: Record<string, Tone> = {
  succeeded: "success",
  failed: "danger",
  loading: "info",
  interrupted: "warning"
};

function captureStatusTone(status: LoadCaptureAttempt["status"]): Tone {
  return lookupOr(captureStatusTones, status, "neutral");
}

function renderLoadCaptureDetail(): void {
  const detail = state.loadCaptures.detail;
  if (!detail) {
    setHTML(insightsElements.loadCaptureDetail, emptyHTML);
    insightsElements.loadCaptureOutput.textContent = "";
    insightsElements.loadCaptureOutputMoreButton.hidden = true;
    return;
  }
  const assets = detail.assets.map(asset => `${asset.role}[${asset.position}] sha256:${asset.sha256}`).join("\n");
  setHTML(insightsElements.loadCaptureDetail, html`
    ${detail.attempt.failure_message ? html`<h3>Failure</h3><pre class="console error-text">${detail.attempt.failure_class + ": " + detail.attempt.failure_message}</pre>` : ""}
    <h3>Sanitized KCPPS</h3><pre class="console">${JSON.stringify(detail.kcpps, null, 2)}</pre>
    <h3>Model hashes</h3><pre class="console">${assets || "None"}</pre>
  `);
  insightsElements.loadCaptureOutput.textContent = combinedOutput();
  insightsElements.loadCaptureOutputMoreButton.hidden = !state.loadCaptures.outputMore;
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
