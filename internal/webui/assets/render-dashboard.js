import { state } from "./state.js";
import { elements } from "./elements.js";
import { renderConstructor } from "./constructor.js";
import {
  allNodeFiles,
  allNodeModels,
  componentOption,
  fileOption,
  filteredFiles,
  filteredModels
} from "./data.js";
import {
  capabilities,
  chip,
  escapeAttribute,
  escapeHTML,
  fileRoles,
  formatBytes,
  optionSummary,
  statusItem
} from "./utils.js";

export function showLogin() {
  elements.loginView.classList.remove("hidden");
  elements.appView.classList.add("hidden");
}

export function showApp() {
  elements.loginView.classList.add("hidden");
  elements.appView.classList.remove("hidden");
}

export function renderInventory() {
  renderNodes();
  renderTables();
  renderCookSelectors();
  renderConstructor();
  renderRecipes();
}

export function renderRouterStatus() {
  const router = state.router || {};
  elements.routerSummary.textContent = `${router.url || ""} ${router.running ? "running" : "stopped"}`;
  elements.launchButton.disabled = !router.managed || router.running;
  elements.restartButton.disabled = !router.managed;
  elements.killButton.disabled = !router.managed || !router.running;
  elements.routerStatus.innerHTML = [
    statusItem("Managed", router.managed ? "yes" : "no"),
    statusItem("Running", router.running ? "yes" : "no"),
    statusItem("URL", router.url || "unknown"),
    statusItem("PID", router.pid || "none"),
    statusItem("Last error", router.error || "none")
  ].join("");
}

export function renderTables() {
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
      <td>${model.available ? "yes" : "no"}</td>
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

export function renderRecipes() {
  const recipes = state.inventory?.recipes || [];
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

function renderNodes() {
  const nodes = state.inventory?.nodes || [];
  elements.nodeCount.textContent = `${nodes.length} nodes`;
  elements.nodesGrid.innerHTML = nodes.map(node => {
    const hardware = node.hardware || {};
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

function renderCookSelectors() {
  const models = allNodeModels();
  const files = allNodeFiles();
  fillSelect(elements.llmSelect, [
    optionValue("", "None"),
    ...models.filter(model => model.has_llm).map(model => componentOption("text", model)),
    ...files.filter(file => fileRoles(file).includes("llm")).map(file => fileOption("text", file))
  ]);
  fillSelect(elements.imageSelect, [
    optionValue("", "None"),
    ...models.filter(model => model.has_image).map(model => componentOption("image", model)),
    ...files.filter(file => fileRoles(file).includes("image")).map(file => fileOption("image", file))
  ]);
  fillSelect(elements.embeddingSelect, [
    optionValue("", "None"),
    ...models.filter(model => model.has_embeddings).map(model => componentOption("embeddings", model)),
    ...files.filter(file => fileRoles(file).includes("embeddings")).map(file => fileOption("embeddings", file))
  ]);
}

function fillSelect(select, options) {
  const selected = select.value;
  select.innerHTML = options.map(option => `<option value="${escapeAttribute(option.value)}">${escapeHTML(option.label)}</option>`).join("");
  if ([...select.options].some(option => option.value === selected)) {
    select.value = selected;
  }
}

function optionValue(value, label) {
  return {value, label};
}
