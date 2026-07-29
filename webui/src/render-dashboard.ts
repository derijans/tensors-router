import { state } from "./state";
import { elements } from "./elements";
import { renderAnalytics } from "./analytics";
import { renderConstructor } from "./constructor";
import { benchmarkCompactLabel } from "./benchmark-data";
import { renderBenchmarks } from "./benchmarks";
import { renderSimpleCook } from "./simple-cook";
import type { Model } from "./types";
import { changedNodeSelection, defaultNodeSelection, retainedNodeSelection } from "./model-filter-data";
import {
  filteredFiles,
  filteredModels
} from "./data";
import {
  capabilities,
  chip,
  escapeAttribute,
  escapeHTML,
  fileRoles,
  formatBytes,
  optionSummary,
  statusItem
} from "./utils";

export function showLogin(): void {
  elements.loginView.classList.remove("hidden");
  elements.appView.classList.add("hidden");
}

export function showApp(): void {
  elements.loginView.classList.add("hidden");
  elements.appView.classList.remove("hidden");
}

export function renderInventory(): void {
  renderNodes();
  renderTables();
  renderBenchmarks();
  renderAnalytics();
  renderSimpleCook();
  renderConstructor();
  renderRecipes();
}

export function renderRouterStatus(): void {
  const router = state.router;
  elements.routerSummary.textContent = `${router?.url || ""} ${router?.running ? "running" : "stopped"}`;
  elements.launchButton.disabled = !router?.managed || Boolean(router?.running);
  elements.restartButton.disabled = !router?.managed;
  elements.shutdownButton.disabled = !router?.can_shutdown;
  elements.forceKillButton.disabled = !router?.can_force_kill;
  elements.routerStatus.innerHTML = [
    statusItem("Managed", router?.managed ? "yes" : "no"),
    statusItem("Running", router?.running ? "yes" : "no"),
    statusItem("URL", router?.url || "unknown"),
    statusItem("PID", router?.pid ? String(router.pid) : "none"),
    statusItem("Can shutdown", router?.can_shutdown ? "yes" : "no"),
    statusItem("Can force kill", router?.can_force_kill ? "yes" : "no"),
    statusItem("Last error", router?.error || "none")
  ].join("");
}

export function renderTables(): void {
  const query = elements.filterInput.value.trim().toLowerCase();
  const nodes = state.inventory?.nodes ?? [];
  syncModelNodeFilters(nodes.map(node => node.node_id));
  const models = filteredModels(query, state.models.configNodeIDs);
  const files = filteredFiles(query, state.models.fileNodeIDs);
  renderNodeFilter(elements.modelsNodeFilter, nodes.map(node => node.node_id), state.models.configNodeIDs);
  renderNodeFilter(elements.filesNodeFilter, nodes.map(node => node.node_id), state.models.fileNodeIDs);
  elements.modelsTable.innerHTML = models.map(model => `
    <tr>
      <td>${escapeHTML(model.public_id || model.local_id)}</td>
      <td>${escapeHTML(model.node_id || "")}</td>
      <td>${escapeHTML(model.backend_mode || "")}</td>
      <td>${escapeHTML(capabilities(model))}</td>
      <td>${escapeHTML(optionSummary(model.options))}</td>
      <td>${escapeHTML(benchmarkCompactLabel(model))}</td>
      <td>${modelAssetAvailability(model)}</td>
      <td>
        <button type="button" data-operation-group="webui" data-load-config="${escapeAttribute(model.public_id || model.local_id)}">Load</button>
      </td>
    </tr>
  `).join("");
  elements.filesTable.innerHTML = files.map(file => `
    <tr>
      <td title="${escapeAttribute(file.path)}">${escapeHTML(file.basename)}</td>
      <td>${escapeHTML(file.node_id || "")}</td>
      <td>${escapeHTML(fileRoles(file).join(", "))}</td>
      <td>${formatBytes(file.size || 0)}</td>
      <td>${fileHashCell(file.node_id || "", file.path, file.sha256 || "")}</td>
    </tr>
  `).join("");
}

export function updateConfigNodeFilter(values: string[]): void {
  state.models.configNodeIDs = changedNodeSelection(values, state.models.configNodeIDs);
  renderTables();
}

export function updateFileNodeFilter(values: string[]): void {
  state.models.fileNodeIDs = changedNodeSelection(values, state.models.fileNodeIDs);
  renderTables();
}

function syncModelNodeFilters(nodeIDs: string[]): void {
  const localNodeID = state.inventory?.node_id || "";
  if (!state.models.initialized) {
    state.models.configNodeIDs = defaultNodeSelection(localNodeID, nodeIDs);
    state.models.fileNodeIDs = defaultNodeSelection(localNodeID, nodeIDs);
    state.models.initialized = true;
    return;
  }
  state.models.configNodeIDs = retainedNodeSelection(state.models.configNodeIDs, localNodeID, nodeIDs);
  state.models.fileNodeIDs = retainedNodeSelection(state.models.fileNodeIDs, localNodeID, nodeIDs);
}

function renderNodeFilter(select: HTMLSelectElement, nodeIDs: string[], selected: string[]): void {
  select.innerHTML = `<option value="*"${selected.includes("*") ? " selected" : ""}>All Nodes</option>${nodeIDs.map(nodeID => `<option value="${escapeAttribute(nodeID)}"${selected.includes(nodeID) ? " selected" : ""}>${escapeHTML(nodeID)}</option>`).join("")}`;
}

function fileHashCell(nodeID: string, path: string, hash: string): string {
  if (!hash) {
    return `<button type="button" data-operation-group="models" data-hash-file-node="${escapeAttribute(nodeID)}" data-hash-file-path="${escapeAttribute(path)}">Hash</button>`;
  }
  return `<span title="${escapeAttribute(hash)}"><code>${escapeHTML(hash.slice(0, 5))}</code> <button type="button" data-copy-file-hash="${escapeAttribute(hash)}">Copy</button></span>`;
}

function modelAssetAvailability(model: Model): string {
  let label = model.available ? "ready" : "unavailable";
  let state = model.available ? "ready" : "failed";
  if (model.asset_state === "unresolved") {
    label = `unresolved (${model.unresolved_fields ?? 0})`;
    state = "unresolved";
  }
  if (model.asset_state === "failed" || model.asset_state === "resolving") {
    label = model.asset_failure ? `${model.asset_state}: ${model.asset_failure}` : model.asset_state;
    state = model.asset_state;
  }
  return `<span class="asset-badge asset-${escapeAttribute(state)}">${escapeHTML(label)}</span>`;
}

export function renderRecipes(): void {
  const recipes = state.inventory?.recipes ?? [];
  elements.recipeCount.textContent = `${recipes.length} recipes`;
  elements.recipesList.innerHTML = recipes.map(recipe => `
    <article class="recipe-item">
      <div>
        <strong>${escapeHTML(recipe.public_id || recipe.id)}</strong>
        <div class="muted">${escapeHTML(recipe.public_image_id || "")}</div>
      </div>
      <button type="button" data-delete-recipe="${escapeAttribute(recipe.id)}">Delete</button>
    </article>
  `).join("");
}

function renderNodes(): void {
  const nodes = state.inventory?.nodes ?? [];
  elements.nodeCount.textContent = `${nodes.length} nodes`;
  elements.nodesGrid.innerHTML = nodes.map(node => {
    const hardware = node.hardware;
    return `
      <article class="node-card">
        <strong>${escapeHTML(node.node_id || node.node_url || "unknown")}</strong>
        <div class="muted">${escapeHTML(node.node_url || "local")}</div>
        <div class="node-meta">
          ${chip(node.backend_mode || "unknown", "cyan")}
          ${chip(node.available ? "available" : "down", node.available ? "lime" : "amber")}
          ${chip(`${hardware.max_threads || "?"} threads`, "magenta")}
          ${chip(`${hardware.gpu_backend || "unknown"} gpu`, "cyan")}
        </div>
        ${node.error ? `<div class="error-text">${escapeHTML(node.error)}</div>` : ""}
      </article>
    `;
  }).join("");
}
