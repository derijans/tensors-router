import { SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import { badge, badgeRow, statusDot } from "../../ui/badge";
import { memoryBar } from "../../ui/charts/chart-markup";
import { memoryKindLabel, nodeMemorySummary } from "./node-memory-data";
import { renderDeviceMemory } from "./node-memory-devices-view";
import { webUIForRuntime } from "../webuis/webui-data";
import { requestsForUnloadedModels, runningRequestCount } from "./node-requests-data";
import { renderRequestStateBadges } from "./node-requests-view";
import type { NodeInventory, NodeRequest, NodeState, NodeStateModelRow, Tone, WebUIEntry } from "../../types";
import { formatGigabytes } from "../../lib/format-bytes";
import { laneAccent } from "../../domain/lanes";
import { errorText, mutedText } from "../../ui/text";
import { iconButton, linkButton } from "../../ui/buttons";
import { lookupOr } from "../../lib/collections";
import { apiCertificateWarning } from "../security/security-api-certificate-data";

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
          ${mutedText(node.node_url || "local", "p")}
        </div>
        ${node.available ? statusDot("Online", "success") : statusDot("Down", "danger")}
      </header>
      ${badgeRow([
        badge(node.role || "unknown", roleTone(node.role)),
        badge(node.backend_mode || "unknown", "info"),
        badge(`${node.hardware.gpu_backend || "unknown"} gpu`, "neutral"),
        badge(`${node.hardware.max_threads || "?"} threads`, "neutral"),
        renderBuildVersion(node, context.clusterBuildVersion),
        renderApiCertificateWarning(node, new Date())
      ])}
      ${node.error ? errorText(node.error) : emptyHTML}
      ${renderMemory(context.snapshot)}
      ${renderRuntimes(node.node_id, context)}
      ${renderCardFooter(node.node_id, context)}
    </article>
  `;
}

const roleTones: Record<string, Tone> = {
  master: "accent",
  slave: "info"
};

function roleTone(role: string): Tone {
  return lookupOr(roleTones, role, "neutral");
}

function renderBuildVersion(node: NodeInventory, buildVersion: string): SafeHTML {
  const version = node.build_version || "unknown build";
  const mismatched = buildVersion !== "" && node.build_version !== buildVersion;
  return mismatched ? badge(`${version} ≠ ${buildVersion}`, "warning") : badge(version, "neutral");
}

function renderApiCertificateWarning(node: NodeInventory, now: Date): SafeHTML {
  const warning = apiCertificateWarning(node.api_certificate, now);
  return warning ? badge(warning.label, warning.tone) : emptyHTML;
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
    ${renderDeviceMemory(snapshot.memory)}
  `;
}

function renderRuntimes(nodeID: string, context: NodeCardContext): SafeHTML {
  if (!context.snapshot) {
    return context.placement === "overview" ? mutedText("Waiting for runtime state…", "p", "runtime-empty") : emptyHTML;
  }
  const runtimes: LoadedRuntime[] = context.snapshot.backends.flatMap(backend => backend.loaded_models.map(row => ({backendID: backend.id, row})));
  const requests = context.snapshot.requests ?? null;
  const activeRequests = context.snapshot.active_requests ?? [];
  const loadedModelIDs = new Set(runtimes.map(runtime => runtime.row.model_id));
  const unloadedModelRows = requests === null ? emptyHTML : renderUnloadedModelRequests(requests, loadedModelIDs);
  if (runtimes.length === 0 && (requests === null || requests.length === 0)) {
    return mutedText("Nothing loaded.", "p", "runtime-empty");
  }
  const loadedRows = html`${runtimes.map(runtime => renderRuntime(nodeID, runtime, activeCount(runtime.row.model_id, requests, activeRequests), context))}`;
  return html`<ul class="runtime-list">${loadedRows}${unloadedModelRows}</ul>`;
}

function activeCount(modelID: string, requests: readonly NodeRequest[] | null, activeRequests: readonly string[]): number {
  return requests === null ? activeRequests.filter(activeModelID => activeModelID === modelID).length : runningRequestCount(requests, modelID);
}

function renderUnloadedModelRequests(requests: readonly NodeRequest[], loadedModelIDs: ReadonlySet<string>): SafeHTML {
  const groups = requestsForUnloadedModels(requests, loadedModelIDs);
  return html`${groups.map(group => html`
    <li class="runtime-row">
      <span class="lane-dot ${laneAccent(group.lane)}"></span>
      <div class="runtime-name">
        <code>${group.modelID}</code>
        <small>${group.lane} · not loaded</small>
      </div>
      ${renderRequestStateBadges(group)}
    </li>
  `)}`;
}

function renderRuntime(nodeID: string, runtime: LoadedRuntime, active: number, context: NodeCardContext): SafeHTML {
  const {row, backendID} = runtime;
  const menuID = runtimeMenuID(context.placement, nodeID, backendID, row.runtime_id);
  const webui = webUIForRuntime(context.webuis, nodeID, row.model_id);
  return html`
    <li class="runtime-row">
      <span class="lane-dot ${row.borrowed ? "lane-lent" : laneAccent(row.lane)}"></span>
      <div class="runtime-name">
        <code>${row.model_id}</code>
        <small>${row.lane} · ${row.runtime_id}${memoryEstimateSuffix(row)}</small>
      </div>
      ${runtimeActivityBadge(row, active)}
      ${iconButton(html`<svg class="icon"><use href="#icon-more"/></svg>`, `Actions for ${row.model_id}`, {popovertarget: menuID})}
      <div id="${menuID}" class="menu" popover>
        <button type="button" data-runtime-action="unload" data-node-id="${nodeID}" data-backend-id="${backendID}" data-runtime-id="${row.runtime_id}" data-generation="${row.generation}">Unload</button>
        <button type="button" data-runtime-action="benchmark" data-node-id="${nodeID}" data-model-id="${row.model_id}">Benchmark</button>
        ${webui ? html`<button type="button" data-runtime-action="webui" data-webui-id="${webui.id}">Open WebUI</button>` : emptyHTML}
        <button type="button" data-runtime-action="edit" data-node-id="${nodeID}" data-model-id="${row.model_id}">Edit config</button>
      </div>
    </li>
  `;
}

function memoryEstimateSuffix(row: NodeStateModelRow): string {
  const estimate = row.memory_estimate_mb ?? 0;
  return estimate > 0 ? ` · ~${formatGigabytes(estimate)} est.` : "";
}

function runtimeActivityBadge(row: NodeStateModelRow, active: number): SafeHTML {
  if (row.borrowed) {
    return badge("lent", "warning");
  }
  return active > 0 ? badge(`${active} active`, "info") : badge("idle", "neutral");
}

function renderCardFooter(nodeID: string, context: NodeCardContext): SafeHTML {
  if (context.placement === "overview") {
    return html`<footer class="node-card-footer">${linkButton("Backends and requests", {"data-node-open": nodeID})}</footer>`;
  }
  return html`
    <footer class="node-card-footer">
      <button type="button" data-node-select="${nodeID}" aria-expanded="${context.expanded}" aria-controls="${nodeStatePanelID(nodeID)}">${context.expanded ? "Hide backends" : "Backends and requests"}</button>
    </footer>
  `;
}

function runtimeMenuID(placement: NodeCardPlacement, nodeID: string, backendID: string, runtimeID: string): string {
  const runtimeKey = [nodeID, backendID, runtimeID].join("-");
  return `runtime-menu-${placement}-${Array.from(runtimeKey).map(menuIDCharacter).join("")}`;
}

function menuIDCharacter(character: string): string {
  if (/[A-Za-z0-9_-]/.test(character)) {
    return character;
  }
  return `_${character.codePointAt(0)?.toString(16) ?? ""}`;
}

