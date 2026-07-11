import { state } from "./state";
import { elements } from "./elements";
import { renderAnalytics } from "./analytics";
import { renderConstructor } from "./constructor";
import { benchmarkCompactLabel } from "./benchmark-data";
import { renderBenchmarks } from "./benchmarks";
import { renderSimpleCook } from "./simple-cook";
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
  const models = filteredModels(query);
  const files = filteredFiles(query);
  elements.modelsTable.innerHTML = models.map(model => `
    <tr>
      <td>${escapeHTML(model.public_id || model.local_id)}</td>
      <td>${escapeHTML(model.node_id || "")}</td>
      <td>${escapeHTML(model.backend_mode || "")}</td>
      <td>${escapeHTML(capabilities(model))}</td>
      <td>${escapeHTML(optionSummary(model.options))}</td>
      <td>${escapeHTML(benchmarkCompactLabel(model))}</td>
      <td>${model.available ? "yes" : "no"}</td>
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
    </tr>
  `).join("");
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
