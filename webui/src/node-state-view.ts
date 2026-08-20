import type { NodeInventory, NodeState, NodeStateBackend, NodeStateModelRow } from "./types";
import { chip, escapeAttribute, escapeHTML, formatBytes } from "./utils";

const backendOrder = ["koboldcpp", "llama-server", "vllm", "sd-server", "whisper-server"];

export function nodeStatePanelID(nodeID: string): string {
  return `nodeStatePanel-${nodeID}`;
}

export function renderNodeCard(node: NodeInventory, expanded: boolean): string {
  const hardware = node.hardware;
  const nodeID = node.node_id || node.node_url || "unknown";
  return `
    <button class="node-card${expanded ? " selected" : ""}" type="button" data-node-select="${escapeAttribute(node.node_id)}" aria-expanded="${expanded}" aria-controls="${escapeAttribute(nodeStatePanelID(node.node_id))}">
      <strong>${escapeHTML(nodeID)}</strong>
      <span class="muted">${escapeHTML(node.node_url || "local")}</span>
      <span class="node-meta">
        ${chip(node.role || "unknown", roleColor(node.role))}
        ${chip(node.source || "unknown", "violet")}
        ${chip(node.backend_mode || "unknown", "cyan")}
        ${chip(node.available ? "available" : "down", node.available ? "lime" : "amber")}
        ${chip(`${hardware.max_threads || "?"} threads`, "magenta")}
        ${chip(`${hardware.gpu_backend || "unknown"} gpu`, "cyan")}
      </span>
      ${node.error ? `<span class="error-text">${escapeHTML(node.error)}</span>` : ""}
    </button>
  `;
}

function roleColor(role: string): string {
  if (role === "master") {
    return "amber";
  }
  if (role === "slave") {
    return "cyan";
  }
  return "magenta";
}

export function renderNodeStateSnapshot(nodeID: string, snapshot: NodeState, pendingUnload: string, pendingBackendAction = ""): string {
  const backends = [...(snapshot.backends || [])].sort((left, right) => backendRank(left.id) - backendRank(right.id));
  return `
    <div class="node-state-backends">
      ${backends.length > 0 ? backends.map(backend => renderBackend(nodeID, backend, pendingUnload, pendingBackendAction)).join("") : `<p class="muted node-state-empty">No backend binaries detected.</p>`}
    </div>
    <section class="node-active-requests" aria-label="Active requests">
      <h4>Active requests</h4>
      ${snapshot.active_requests.length > 0 ? `<ul>${snapshot.active_requests.map(modelID => `<li>${escapeHTML(modelID)}</li>`).join("")}</ul>` : `<p class="muted node-state-empty">No active requests.</p>`}
    </section>
  `;
}

function renderBackend(nodeID: string, backend: NodeStateBackend, pendingUnload: string, pendingBackendAction: string): string {
  const initializationPending = pendingBackendAction === backendActionKey("init", backend.id);
  const cancellationPending = pendingBackendAction === backendActionKey("cancel", backend.id);
  const lifecycleState = initializationPending ? "initializing" : backend.lifecycle_state || "ready";
  return `
    <article class="node-state-backend">
      <div class="node-backend-heading">
        <h4>${escapeHTML(backend.display_name)}</h4>
        <div class="node-backend-chips">
          ${chip(backend.mode, "cyan")}
          ${chip(lifecycleState, lifecycleColor(lifecycleState))}
        </div>
      </div>
      ${lifecycleState === "ready" ? renderReadyBackend(nodeID, backend, pendingUnload) : renderBackendLifecycle(nodeID, backend, lifecycleState, cancellationPending)}
    </article>
  `;
}

function renderReadyBackend(nodeID: string, backend: NodeStateBackend, pendingUnload: string): string {
  return `
    ${renderRuntimeIdentity(backend)}
    ${backend.loaded_models.length > 0 ? `<div class="node-loaded-models">${backend.loaded_models.map(model => renderLoadedModel(nodeID, backend.id, model, pendingUnload)).join("")}</div>` : `<p class="muted node-state-empty">No loaded models.</p>`}
  `;
}

function renderBackendLifecycle(nodeID: string, backend: NodeStateBackend, lifecycleState: string, cancellationPending: boolean): string {
  if (lifecycleState === "initializing") {
    return `
      ${renderRuntimeIdentity(backend)}
      <div class="node-backend-lifecycle">
        <strong>${escapeHTML(backend.initialization_phase || "Initializing")}</strong>
        ${renderInitializationProgress(backend)}
        <div class="node-backend-actions">
          <button class="chip amber node-backend-init-action" type="button" disabled>backend needs init</button>
          <button type="button" data-node-backend-init-cancel data-node-id="${escapeAttribute(nodeID)}" data-backend-id="${escapeAttribute(backend.id)}"${cancellationPending ? " disabled" : ""}>${cancellationPending ? "Cancelling..." : "Cancel"}</button>
        </div>
      </div>
    `;
  }
  const reason = backend.error || lifecycleReason(lifecycleState);
  const initializationAction = lifecycleState === "needs_init" || (lifecycleState === "failed" && backend.retryable);
  return `
    ${renderRuntimeIdentity(backend)}
    <div class="node-backend-lifecycle">
      ${reason ? `<p class="${lifecycleState === "failed" ? "error-text" : "muted"} node-state-message">${escapeHTML(reason)}</p>` : ""}
      ${initializationAction ? `<button class="chip amber node-backend-init-action" type="button" data-node-backend-init data-node-id="${escapeAttribute(nodeID)}" data-backend-id="${escapeAttribute(backend.id)}"${backend.selected_profile ? ` data-profile="${escapeAttribute(backend.selected_profile)}"` : ""}>backend needs init</button>` : ""}
    </div>
  `;
}

function renderRuntimeIdentity(backend: NodeStateBackend): string {
  const rows = [
    backend.runtime_version ? `Version: ${backend.runtime_version}` : "",
    backend.selected_profile ? `Profile: ${backend.selected_profile}` : "",
    backend.detected_profile && backend.detected_profile !== backend.selected_profile ? `Detected: ${backend.detected_profile}` : "",
    backend.manifest_trust && backend.manifest_trust !== "tuf" ? `Manifest trust: ${backend.manifest_trust}` : ""
  ].filter(Boolean);
  return rows.length > 0 ? `<div class="muted node-backend-runtime">${rows.map(value => `<span>${escapeHTML(value)}</span>`).join("")}</div>` : "";
}

function renderInitializationProgress(backend: NodeStateBackend): string {
  const completedBytes = positiveBytes(backend.initialization_bytes);
  const totalBytes = positiveBytes(backend.initialization_total_bytes);
  if (totalBytes === 0) {
    const label = completedBytes > 0 ? `${formatBytes(completedBytes)} completed` : "Waiting for progress";
    return `<progress class="node-backend-progress" aria-label="Initialization progress"></progress><span class="muted">${escapeHTML(label)}</span>`;
  }
  const boundedCompleted = Math.min(completedBytes, totalBytes);
  const percent = Math.floor((boundedCompleted / totalBytes) * 100);
  const label = `${formatBytes(completedBytes)} / ${formatBytes(totalBytes)} (${percent}%)`;
  return `<progress class="node-backend-progress" aria-label="Initialization progress" value="${boundedCompleted}" max="${totalBytes}"></progress><span class="muted">${escapeHTML(label)}</span>`;
}

function positiveBytes(value: number | undefined): number {
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? Math.floor(value) : 0;
}

function lifecycleReason(lifecycleState: string): string {
  if (lifecycleState === "companion_missing") {
    return "vLLM companion is missing.";
  }
  if (lifecycleState === "unsupported") {
    return "vLLM is unsupported on this platform.";
  }
  return "";
}

function lifecycleColor(lifecycleState: string): string {
  if (lifecycleState === "ready") {
    return "lime";
  }
  if (lifecycleState === "failed" || lifecycleState === "unsupported" || lifecycleState === "companion_missing") {
    return "amber";
  }
  return "violet";
}

function backendActionKey(action: string, backendID: string): string {
  return `${action} ${backendID}`;
}

function renderLoadedModel(nodeID: string, backendID: string, model: NodeStateModelRow, pendingUnload: string): string {
  const pending = pendingUnload === `${backendID} ${model.runtime_id}`;
  return `
    <div class="node-loaded-model">
      <div>
        <strong>${escapeHTML(model.model_id)}</strong>
        <div class="muted">${escapeHTML(model.lane)} / ${escapeHTML(model.runtime_id)}</div>
      </div>
      <button type="button" data-node-unload data-node-id="${escapeAttribute(nodeID)}" data-backend-id="${escapeAttribute(backendID)}" data-runtime-id="${escapeAttribute(model.runtime_id)}" data-generation="${model.generation}"${pending ? " disabled" : ""}>${pending ? "Unloading..." : "Unload"}</button>
    </div>
  `;
}

function backendRank(backendID: string): number {
  const rank = backendOrder.indexOf(backendID);
  return rank === -1 ? backendOrder.length : rank;
}
