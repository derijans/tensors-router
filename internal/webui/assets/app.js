const state = {
  csrf: "",
  inventory: null,
  router: null
};

const elements = {
  loginView: document.getElementById("loginView"),
  appView: document.getElementById("appView"),
  loginForm: document.getElementById("loginForm"),
  tokenInput: document.getElementById("tokenInput"),
  loginError: document.getElementById("loginError"),
  logoutButton: document.getElementById("logoutButton"),
  refreshButton: document.getElementById("refreshButton"),
  launchButton: document.getElementById("launchButton"),
  restartButton: document.getElementById("restartButton"),
  killButton: document.getElementById("killButton"),
  routerSummary: document.getElementById("routerSummary"),
  routerStatus: document.getElementById("routerStatus"),
  nodeCount: document.getElementById("nodeCount"),
  nodesGrid: document.getElementById("nodesGrid"),
  filterInput: document.getElementById("filterInput"),
  modelsTable: document.getElementById("modelsTable"),
  filesTable: document.getElementById("filesTable"),
  cookForm: document.getElementById("cookForm"),
  cookIdInput: document.getElementById("cookIdInput"),
  overwriteInput: document.getElementById("overwriteInput"),
  llmSelect: document.getElementById("llmSelect"),
  imageSelect: document.getElementById("imageSelect"),
  embeddingSelect: document.getElementById("embeddingSelect"),
  previewButton: document.getElementById("previewButton"),
  cookOutput: document.getElementById("cookOutput"),
  recipeCount: document.getElementById("recipeCount"),
  recipesList: document.getElementById("recipesList")
};

async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (options.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (state.csrf && options.method && options.method !== "GET") {
    headers.set("X-CSRF-Token", state.csrf);
  }
  const response = await fetch(path, {...options, headers});
  const text = await response.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = {raw: text};
    }
  }
  if (!response.ok) {
    throw new Error(data?.error || data?.error?.message || text || response.statusText);
  }
  return data;
}

async function bootstrap() {
  try {
    const session = await api("/api/session");
    state.csrf = session.csrf;
    showApp();
    await refreshAll();
  } catch {
    showLogin();
  }
}

function showLogin() {
  elements.loginView.classList.remove("hidden");
  elements.appView.classList.add("hidden");
}

function showApp() {
  elements.loginView.classList.add("hidden");
  elements.appView.classList.remove("hidden");
}

async function refreshAll() {
  await refreshRouterStatus();
  await refreshInventory();
}

async function refreshRouterStatus() {
  state.router = await api("/api/router/status");
  renderRouterStatus();
}

async function refreshInventory() {
  state.inventory = await api("/api/inventory");
  renderInventory();
}

function renderRouterStatus() {
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

function renderInventory() {
  const inventory = state.inventory || {nodes: [], models: [], recipes: []};
  const nodes = inventory.nodes || [];
  elements.nodeCount.textContent = `${nodes.length} nodes`;
  elements.nodesGrid.innerHTML = nodes.map(node => `
    <article class="node-card">
      <strong>${escapeHTML(node.node_id || node.node_url || "unknown")}</strong>
      <div class="muted">${escapeHTML(node.role || "")} ${escapeHTML(node.source || "")}</div>
      <div>${escapeHTML(node.node_url || "")}</div>
      <div>${escapeHTML(node.backend_mode || "unknown")} · ${node.available ? "available" : "down"}</div>
      ${node.error ? `<div class="error-text">${escapeHTML(node.error)}</div>` : ""}
    </article>
  `).join("");
  renderTables();
  renderCookSelectors();
  renderRecipes();
}

function renderTables() {
  const query = elements.filterInput.value.trim().toLowerCase();
  const models = filteredModels(query);
  const files = filteredFiles(query);
  elements.modelsTable.innerHTML = models.map(model => `
    <tr>
      <td>${escapeHTML(model.public_id || model.local_id)}</td>
      <td>${escapeHTML(model.node_id || "")}</td>
      <td>${escapeHTML(model.backend_mode || "")}</td>
      <td>${escapeHTML(capabilities(model))}</td>
      <td>${model.available ? "yes" : "no"}</td>
    </tr>
  `).join("");
  elements.filesTable.innerHTML = files.map(file => `
    <tr>
      <td title="${escapeHTML(file.path)}">${escapeHTML(file.basename)}</td>
      <td>${escapeHTML(file.node_id || "")}</td>
      <td>${escapeHTML((file.roles || [file.role || "unknown"]).join(", "))}</td>
      <td>${formatBytes(file.size || 0)}</td>
    </tr>
  `).join("");
}

function renderCookSelectors() {
  const models = allNodeModels();
  const files = allNodeFiles();
  fillSelect(elements.llmSelect, [
    optionValue("", "None"),
    ...models.filter(model => model.has_llm).map(model => componentOption("text", model)),
    ...files.filter(file => (file.roles || []).includes("llm") || file.role === "llm").map(file => fileOption("text", file))
  ]);
  fillSelect(elements.imageSelect, [
    optionValue("", "None"),
    ...models.filter(model => model.has_image).map(model => componentOption("image", model)),
    ...files.filter(file => (file.roles || []).includes("image") || file.role === "image").map(file => fileOption("image", file))
  ]);
  fillSelect(elements.embeddingSelect, [
    optionValue("", "None"),
    ...models.filter(model => model.has_embeddings).map(model => componentOption("embeddings", model)),
    ...files.filter(file => (file.roles || []).includes("embeddings")).map(file => fileOption("embeddings", file))
  ]);
}

function renderRecipes() {
  const recipes = state.inventory?.recipes || [];
  elements.recipeCount.textContent = `${recipes.length} recipes`;
  elements.recipesList.innerHTML = recipes.map(recipe => `
    <article class="recipe-item">
      <div>
        <strong>${escapeHTML(recipe.public_id || recipe.id)}</strong>
        <div class="muted">${escapeHTML(recipe.public_image_id || "")}</div>
      </div>
      <button type="button" data-delete-recipe="${escapeHTML(recipe.id)}">Delete</button>
    </article>
  `).join("");
}

function filteredModels(query) {
  const models = state.inventory?.models || [];
  if (!query) {
    return models;
  }
  return models.filter(model => JSON.stringify(model).toLowerCase().includes(query));
}

function filteredFiles(query) {
  const files = allNodeFiles();
  if (!query) {
    return files;
  }
  return files.filter(file => JSON.stringify(file).toLowerCase().includes(query));
}

function allNodeModels() {
  return (state.inventory?.nodes || []).flatMap(node => node.models || []);
}

function allNodeFiles() {
  return (state.inventory?.nodes || []).flatMap(node => node.files || []);
}

function componentOption(kind, model) {
  return optionValue(JSON.stringify({
    kind,
    node_id: model.node_id,
    node_url: model.node_url || "",
    source: "config",
    model_id: model.local_id,
    image_id: kind === "image" ? model.image_id : ""
  }), `${model.node_id} · ${kind === "image" ? model.public_image_id || model.image_id : model.public_id || model.local_id}`);
}

function fileOption(kind, file) {
  return optionValue(JSON.stringify({
    kind,
    node_id: file.node_id,
    source: "file",
    file_path: file.path
  }), `${file.node_id} · ${file.basename}`);
}

function optionValue(value, label) {
  return {value, label};
}

function fillSelect(select, options) {
  const selected = select.value;
  select.innerHTML = options.map(option => `<option value="${escapeAttribute(option.value)}">${escapeHTML(option.label)}</option>`).join("");
  if ([...select.options].some(option => option.value === selected)) {
    select.value = selected;
  }
}

function cookRequest() {
  const components = [elements.llmSelect, elements.imageSelect, elements.embeddingSelect]
    .map(select => select.value)
    .filter(Boolean)
    .map(value => JSON.parse(value));
  return {
    id: elements.cookIdInput.value.trim(),
    overwrite: elements.overwriteInput.checked,
    components
  };
}

async function previewCook() {
  const result = await api("/api/cook/preview", {
    method: "POST",
    body: JSON.stringify(cookRequest())
  });
  elements.cookOutput.textContent = JSON.stringify(result, null, 2);
}

async function applyCook() {
  const result = await api("/api/cook/apply", {
    method: "POST",
    body: JSON.stringify(cookRequest())
  });
  elements.cookOutput.textContent = JSON.stringify(result, null, 2);
  await refreshInventory();
}

function statusItem(label, value) {
  return `
    <div class="status-item">
      <div class="status-label">${escapeHTML(label)}</div>
      <div class="status-value">${escapeHTML(String(value))}</div>
    </div>
  `;
}

function capabilities(model) {
  const values = [];
  if (model.has_llm) values.push("llm");
  if (model.has_image) values.push("image");
  if (model.has_embeddings) values.push("embeddings");
  if (model.has_multimodal) values.push("multimodal");
  return values.join(", ") || "none";
}

function formatBytes(value) {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`;
  return `${(value / 1024 / 1024 / 1024).toFixed(1)} GB`;
}

function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>"']/g, character => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    "\"": "&quot;",
    "'": "&#39;"
  }[character]));
}

function escapeAttribute(value) {
  return escapeHTML(value).replace(/`/g, "&#96;");
}

elements.loginForm.addEventListener("submit", async event => {
  event.preventDefault();
  elements.loginError.textContent = "";
  try {
    const session = await api("/api/login", {
      method: "POST",
      body: JSON.stringify({token: elements.tokenInput.value})
    });
    state.csrf = session.csrf;
    showApp();
    await refreshAll();
  } catch (error) {
    elements.loginError.textContent = error.message;
  }
});

elements.logoutButton.addEventListener("click", async () => {
  await api("/api/logout", {method: "POST"});
  state.csrf = "";
  showLogin();
});

elements.refreshButton.addEventListener("click", refreshAll);
elements.filterInput.addEventListener("input", renderTables);
elements.launchButton.addEventListener("click", async () => {
  state.router = await api("/api/router/launch", {method: "POST"});
  renderRouterStatus();
});
elements.restartButton.addEventListener("click", async () => {
  state.router = await api("/api/router/restart", {method: "POST"});
  renderRouterStatus();
});
elements.killButton.addEventListener("click", async () => {
  state.router = await api("/api/router/kill", {method: "POST"});
  renderRouterStatus();
});
elements.previewButton.addEventListener("click", previewCook);
elements.cookForm.addEventListener("submit", async event => {
  event.preventDefault();
  await applyCook();
});
elements.recipesList.addEventListener("click", async event => {
  const id = event.target?.dataset?.deleteRecipe;
  if (!id) {
    return;
  }
  await api(`/api/cook/${encodeURIComponent(id)}`, {method: "DELETE"});
  await refreshInventory();
});

bootstrap();
