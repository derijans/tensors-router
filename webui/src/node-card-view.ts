import { SafeHTML, emptyHTML, html } from "./safe-html";
import { badge, laneAccent } from "./markup-primitives";
import { memoryBar } from "./chart-markup";
import { formatGigabytes, memoryKindLabel, nodeMemorySummary } from "./node-memory-data";
import { webUIForRuntime } from "./webui-data";
import type { NodeInventory, NodeState, NodeStateModelRow, Tone, WebUIEntry } from "./types";

export type NodeCardPlacement = "overview" | "nodes";

export interface NodeCardContext {
  placement: NodeCardPlacement;
  expanded: boolean;
  clusterBuildVersion: string;
  snapshot: NodeState | null;
  webuis: readonly WebUIEntry[];
}

interface LoadedRuntime {
  backendID: string;
  row: NodeStateModelRow;
}

export function nodeStatePanelID(nodeID: string): string {
  return `nodeStatePanel-${nodeID}`;
}

export function nodeCardID(placement: NodeCardPlacement, nodeID: string): string {
  return `nodeCard-${placement}-${nodeID}`;
}

export function nodeDisplayName(node: NodeInventory): string {
  return node.node_id || node.node_url || "unknown node";
}

export function clusterBuildVersion(nodes: readonly NodeInventory[]): string {
  const master = nodes.find(node => node.role === "master") ?? nodes[0];
  return master?.build_version ?? "";
}

export function renderNodeCard(node: NodeInventory, context: NodeCardContext): SafeHTML {
  const nodeID = nodeDisplayName(node);
  return html`
    <article id="${nodeCardID(context.placement, node.node_id)}" class="card node-card${context.expanded ? " selected" : ""}">
      <header class="node-card-head">
        <span class="node-icon"><svg class="icon"><use href="#icon-nodes"/></svg></span>
        <div class="node-card-title">
          <h3>${nodeID}</h3>
          <p class="muted">${node.node_url || "local"}</p>
        </div>
        <span class="status-dot ${node.available ? "tone-success" : "tone-danger"}">${node.available ? "Online" : "Down"}</span>
      </header>
      <div class="badge-row">
        ${badge(node.role || "unknown", roleTone(node.role))}
        ${badge(node.backend_mode || "unknown", "info")}
        ${badge(`${node.hardware.gpu_backend || "unknown"} gpu`, "neutral")}
        ${badge(`${node.hardware.max_threads || "?"} threads`, "neutral")}
        ${renderBuildVersion(node, context.clusterBuildVersion)}
      </div>
      ${node.error ? html`<p class="error-text">${node.error}</p>` : emptyHTML}
      ${renderMemory(context.snapshot)}
      ${renderRuntimes(node.node_id, context)}
      ${renderCardFooter(node.node_id, context)}
    </article>
  `;
}

function roleTone(role: string): Tone {
  if (role === "master") {
    return "accent";
  }
  return role === "slave" ? "info" : "neutral";
}

function renderBuildVersion(node: NodeInventory, buildVersion: string): SafeHTML {
  const version = node.build_version || "unknown build";
  const mismatched = buildVersion !== "" && node.build_version !== buildVersion;
  return mismatched ? badge(`${version} ≠ ${buildVersion}`, "warning") : badge(version, "neutral");
}

function renderMemory(snapshot: NodeState | null): SafeHTML {
  if (!snapshot) {
    return emptyHTML;
  }
  const summary = nodeMemorySummary(snapshot);
  if (!summary) {
    return html`<p class="memory-row muted">Memory unavailable on this node</p>`;
  }
  const label = `${memoryKindLabel(summary.kind)} ${formatGigabytes(summary.usedMB)} of ${formatGigabytes(summary.totalMB)}`;
  return html`
    <div class="memory-row">
      <span>${memoryKindLabel(summary.kind)}</span>
      <b>${formatGigabytes(summary.usedMB)} / ${formatGigabytes(summary.totalMB)}</b>
    </div>
    ${memoryBar(summary.segments, label)}
  `;
}

function renderRuntimes(nodeID: string, context: NodeCardContext): SafeHTML {
  if (!context.snapshot) {
    return context.placement === "overview" ? html`<p class="muted runtime-empty">Waiting for runtime state…</p>` : emptyHTML;
  }
  const runtimes: LoadedRuntime[] = context.snapshot.backends.flatMap(backend => backend.loaded_models.map(row => ({backendID: backend.id, row})));
  if (runtimes.length === 0) {
    return html`<p class="muted runtime-empty">Nothing loaded.</p>`;
  }
  const activeRequests = context.snapshot.active_requests ?? [];
  return html`<ul class="runtime-list">${runtimes.map(runtime => renderRuntime(nodeID, runtime, activeRequests, context))}</ul>`;
}

function renderRuntime(nodeID: string, runtime: LoadedRuntime, activeRequests: readonly string[], context: NodeCardContext): SafeHTML {
  const {row, backendID} = runtime;
  const active = activeRequests.filter(modelID => modelID === row.model_id).length;
  const menuID = runtimeMenuID(context.placement, nodeID, backendID, row.runtime_id);
  const webui = webUIForRuntime(context.webuis, nodeID, row.model_id);
  return html`
    <li class="runtime-row">
      <span class="lane-dot ${row.borrowed ? "lane-lent" : laneAccent(row.lane)}"></span>
      <div class="runtime-name">
        <code>${row.model_id}</code>
        <small>${row.lane} · ${row.runtime_id}${(row.memory_estimate_mb ?? 0) > 0 ? ` · ~${formatGigabytes(row.memory_estimate_mb ?? 0)} est.` : ""}</small>
      </div>
      ${row.borrowed ? badge("lent", "warning") : active > 0 ? badge(`${active} active`, "info") : badge("idle", "neutral")}
      <button class="icon-button" type="button" popovertarget="${menuID}" aria-label="Actions for ${row.model_id}"><svg class="icon"><use href="#icon-more"/></svg></button>
      <div id="${menuID}" class="menu" popover>
        <button type="button" data-runtime-action="unload" data-node-id="${nodeID}" data-backend-id="${backendID}" data-runtime-id="${row.runtime_id}" data-generation="${row.generation}">Unload</button>
        <button type="button" data-runtime-action="benchmark" data-node-id="${nodeID}" data-model-id="${row.model_id}">Benchmark</button>
        ${webui ? html`<button type="button" data-runtime-action="webui" data-webui-id="${webui.id}">Open WebUI</button>` : emptyHTML}
        <button type="button" data-runtime-action="edit" data-node-id="${nodeID}" data-model-id="${row.model_id}">Edit config</button>
      </div>
    </li>
  `;
}

function renderCardFooter(nodeID: string, context: NodeCardContext): SafeHTML {
  if (context.placement === "overview") {
    return html`<footer class="node-card-footer"><button class="link-button" type="button" data-node-open="${nodeID}">Backends and requests</button></footer>`;
  }
  return html`
    <footer class="node-card-footer">
      <button type="button" data-node-select="${nodeID}" aria-expanded="${context.expanded}" aria-controls="${nodeStatePanelID(nodeID)}">${context.expanded ? "Hide backends" : "Backends and requests"}</button>
    </footer>
  `;
}

function runtimeMenuID(placement: NodeCardPlacement, nodeID: string, backendID: string, runtimeID: string): string {
  return `runtime-menu-${placement}-${Array.from(`${nodeID}-${backendID}-${runtimeID}`)
    .map(character => /[A-Za-z0-9_-]/.test(character) ? character : `_${character.codePointAt(0)?.toString(16) ?? ""}`)
    .join("")}`;
}
