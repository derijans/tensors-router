import { api } from "./api.js";
import { elements } from "./elements.js";
import { state } from "./state.js";
import {
  addOption,
  addPayload,
  clearConstructor,
  clearLane,
  renderConstructor,
  removeOption,
  toggleInspectorList,
  updateOptionInput
} from "./constructor.js";
import {
  applyAdvancedCook,
  applyCook,
  previewAdvancedCook,
  previewCook
} from "./cook-actions.js";
import {
  renderInventory,
  renderRecipes,
  renderRouterStatus,
  renderTables,
  showApp,
  showLogin
} from "./render-dashboard.js";

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

function activateTab(name) {
  state.activeTab = name;
  document.querySelectorAll("[data-tab]").forEach(tab => tab.classList.toggle("active", tab.dataset.tab === name));
  document.querySelectorAll("[data-panel]").forEach(panel => panel.classList.toggle("active", panel.dataset.panel === name));
}

function activateCookMode(name) {
  state.activeCookMode = name;
  document.querySelectorAll("[data-cook-mode]").forEach(tab => tab.classList.toggle("active", tab.dataset.cookMode === name));
  document.querySelectorAll("[data-cook-panel]").forEach(panel => panel.classList.toggle("active", panel.dataset.cookPanel === name));
}

function activatePalette(name) {
  state.activePalette = name;
  document.querySelectorAll("[data-palette]").forEach(tab => tab.classList.toggle("active", tab.dataset.palette === name));
  renderConstructor();
}

document.querySelectorAll("[data-tab]").forEach(button => {
  button.addEventListener("click", () => activateTab(button.dataset.tab));
});

document.querySelectorAll("[data-cook-mode]").forEach(button => {
  button.addEventListener("click", () => activateCookMode(button.dataset.cookMode));
});

document.querySelectorAll("[data-palette]").forEach(button => {
  button.addEventListener("click", () => activatePalette(button.dataset.palette));
});

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
elements.constructorFilterInput.addEventListener("input", renderConstructor);

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
  await applyCook(refreshInventory);
});

elements.advancedPreviewButton.addEventListener("click", previewAdvancedCook);
elements.advancedApplyButton.addEventListener("click", () => applyAdvancedCook(refreshInventory));
elements.clearConstructorButton.addEventListener("click", clearConstructor);

elements.paletteList.addEventListener("dragstart", event => {
  const payloadID = event.target.closest("[data-drag-payload]")?.dataset.dragPayload;
  if (!payloadID) {
    return;
  }
  event.dataTransfer.setData("text/plain", payloadID);
  event.dataTransfer.effectAllowed = "copy";
});

elements.paletteList.addEventListener("click", event => {
  const optionKey = event.target?.dataset?.addOption;
  if (optionKey) {
    addOption(optionKey);
    return;
  }
  const payloadID = event.target?.dataset?.selectPayload;
  if (payloadID) {
    addPayload(state.palettePayloads[payloadID]);
  }
});

elements.constructorLanes.addEventListener("dragover", event => {
  const drop = event.target.closest("[data-drop-lane]");
  if (!drop) {
    return;
  }
  event.preventDefault();
  drop.classList.add("drag-over");
});

elements.constructorLanes.addEventListener("dragleave", event => {
  event.target.closest("[data-drop-lane]")?.classList.remove("drag-over");
});

elements.constructorLanes.addEventListener("drop", event => {
  const drop = event.target.closest("[data-drop-lane]");
  if (!drop) {
    return;
  }
  event.preventDefault();
  drop.classList.remove("drag-over");
  addPayload(state.palettePayloads[event.dataTransfer.getData("text/plain")], drop.dataset.dropLane);
});

elements.constructorLanes.addEventListener("click", event => {
  const lane = event.target?.dataset?.clearLane;
  if (lane) {
    clearLane(lane);
  }
});

elements.selectedOptionsList.addEventListener("input", event => updateOptionInput(event.target));
elements.selectedOptionsList.addEventListener("click", event => {
  const removeKey = event.target?.dataset?.removeOption;
  if (removeKey) {
    removeOption(removeKey);
    return;
  }
  const toggle = event.target?.dataset?.toggleList;
  if (toggle) {
    toggleInspectorList(toggle);
  }
});

elements.usedModelsList.addEventListener("click", event => {
  const toggle = event.target?.dataset?.toggleList;
  if (toggle) {
    toggleInspectorList(toggle);
  }
});

elements.recipesList.addEventListener("click", async event => {
  const id = event.target?.dataset?.deleteRecipe;
  if (!id) {
    return;
  }
  await api(`/api/cook/${encodeURIComponent(id)}`, {method: "DELETE"});
  await refreshInventory();
  renderRecipes();
});

bootstrap();
