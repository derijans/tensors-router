import type { NodeInventory, NodeState, NodeStateBackend, NodeStateModelRow } from "./types";
import { chip, escapeAttribute, escapeHTML } from "./utils";

const backendOrder = ["koboldcpp", "llama-server", "sd-server", "whisper-server"];

export function renderNodeCard(node: NodeInventory, selected: boolean): string {
  const hardware = node.hardware;
  const nodeID = node.node_id || node.node_url || "unknown";
  return `
    <button class="node-card${selected ? " selected" : ""}" type="button" data-node-select="${escapeAttribute(node.node_id)}" aria-expanded="${selected}" aria-controls="nodeStatePanel">
      <strong>${escapeHTML(nodeID)}</strong>
      <span class="muted">${escapeHTML(node.node_url || "local")}</span>
      <span class="node-meta">
        ${chip(node.backend_mode || "unknown", "cyan")}
        ${chip(node.available ? "available" : "down", node.available ? "lime" : "amber")}
        ${chip(`${hardware.max_threads || "?"} threads`, "magenta")}
        ${chip(`${hardware.gpu_backend || "unknown"} gpu`, "cyan")}
      </span>
      ${node.error ? `<span class="error-text">${escapeHTML(node.error)}</span>` : ""}
    </button>
  `;
}

export function renderNodeStateSnapshot(snapshot: NodeState, pendingUnload: string): string {
  const backends = [...(snapshot.backends || [])].sort((left, right) => backendRank(left.id) - backendRank(right.id));
  return `
    <div class="node-state-backends">
      ${backends.length > 0 ? backends.map(backend => renderBackend(backend, pendingUnload)).join("") : `<p class="muted node-state-empty">No backend binaries detected.</p>`}
    </div>
    <section class="node-active-requests" aria-label="Active requests">
      <h4>Active requests</h4>
      ${snapshot.active_requests.length > 0 ? `<ul>${snapshot.active_requests.map(modelID => `<li>${escapeHTML(modelID)}</li>`).join("")}</ul>` : `<p class="muted node-state-empty">No active requests.</p>`}
    </section>
  `;
}

function renderBackend(backend: NodeStateBackend, pendingUnload: string): string {
  return `
    <article class="node-state-backend">
      <div class="node-backend-heading">
        <h4>${escapeHTML(backend.display_name)}</h4>
        ${chip(backend.mode, "cyan")}
      </div>
      ${backend.loaded_models.length > 0 ? `<div class="node-loaded-models">${backend.loaded_models.map(model => renderLoadedModel(backend.id, model, pendingUnload)).join("")}</div>` : `<p class="muted node-state-empty">No loaded models.</p>`}
    </article>
  `;
}

function renderLoadedModel(backendID: string, model: NodeStateModelRow, pendingUnload: string): string {
  const pending = pendingUnload === `${backendID}\u0000${model.runtime_id}`;
  return `
    <div class="node-loaded-model">
      <div>
        <strong>${escapeHTML(model.model_id)}</strong>
        <div class="muted">${escapeHTML(model.lane)} / ${escapeHTML(model.runtime_id)}</div>
      </div>
      <button type="button" data-node-unload data-backend-id="${escapeAttribute(backendID)}" data-runtime-id="${escapeAttribute(model.runtime_id)}" data-generation="${model.generation}"${pending ? " disabled" : ""}>${pending ? "Unloading..." : "Unload"}</button>
    </div>
  `;
}

function backendRank(backendID: string): number {
  const rank = backendOrder.indexOf(backendID);
  return rank === -1 ? backendOrder.length : rank;
}
