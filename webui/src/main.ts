import {
  deleteRecipe,
  forceKillRouter,
  getInventory,
  getRouterStatus,
  getSession,
  launchRouter,
  login,
  logout,
  restartRouter,
  shutdownRouter
} from "./api";
import {
  loadSelectedBenchmark,
  runSelectedBenchmark,
  selectBenchmarkModel,
  selectBenchmarkType,
  toggleAllBenchmarkSections,
  updateBenchmarkSections
} from "./benchmarks";
import {
  loadAnalytics,
  updateAnalyticsModel,
  updateAnalyticsNode,
  updateAnalyticsPeriod,
  updateAnalyticsSection
} from "./analytics";
import { closestElement, elementTarget, queryElements } from "./dom";
import { elements } from "./elements";
import { state } from "./state";
import {
  addOption,
  addPayload,
  clearConstructor,
  clearLane,
  editLaneFields,
  removeOption,
  renderConstructor,
  toggleInspectorList,
  updateConstructorBackendMode,
  updateLaneTarget,
  updateOptionInput
} from "./constructor";
import {
  closeFieldEditor,
  handleFieldEditorClick,
  handleFieldEditorInput
} from "./constructor-field-editor";
import {
  applyAdvancedCook,
  previewAdvancedCook
} from "./cook-actions";
import { loadSelectedConfig } from "./model-actions";
import {
  closeWebUIDialog,
  loadSelectedWebUIModel,
  loadWebUIs,
  openSelectedWebUI,
  setWebUIEnabled,
  showSelectedWebUIDialog,
  updateWebUIFilter
} from "./webuis";
import {
  addSelectedSimpleField,
  applySimpleCook,
  copySimpleConfig,
  deleteSimpleConfig,
  newSimpleConfig,
  previewSimpleCook,
  removeSimpleField,
  renderSimpleCook,
  selectSimpleConfig,
  selectSimpleNode,
  showSimpleFieldValues,
  updateSimpleField,
  updateSimpleFieldFilter,
  updateSimpleSectionOpen
} from "./simple-cook";
import {
  renderInventory,
  renderRecipes,
  renderRouterStatus,
  renderTables,
  showApp,
  showLogin
} from "./render-dashboard";
import type { CookMode, PaletteName } from "./types";

async function bootstrap(): Promise<void> {
  try {
    const session = await getSession();
    state.csrf = session.csrf;
    showApp();
    await refreshAll();
  } catch {
    showLogin();
  }
}

async function refreshAll(): Promise<void> {
  await refreshRouterStatus();
  await refreshInventory();
  await loadWebUIs();
  await loadAnalytics();
}

async function refreshRouterStatus(): Promise<void> {
  state.router = await getRouterStatus();
  renderRouterStatus();
}

async function refreshInventory(): Promise<void> {
  state.inventory = await getInventory();
  renderInventory();
}

function activateTab(name: string): void {
  state.activeTab = name;
  queryElements("[data-tab]", HTMLButtonElement).forEach(tab => tab.classList.toggle("active", tab.dataset.tab === name));
  queryElements("[data-panel]", HTMLElement).forEach(panel => panel.classList.toggle("active", panel.dataset.panel === name));
}

function activateCookMode(name: string | undefined): void {
  if (!isCookMode(name)) {
    return;
  }
  state.activeCookMode = name;
  queryElements("[data-cook-mode]", HTMLButtonElement).forEach(tab => tab.classList.toggle("active", tab.dataset.cookMode === name));
  queryElements("[data-cook-panel]", HTMLElement).forEach(panel => panel.classList.toggle("active", panel.dataset.cookPanel === name));
}

function activatePalette(name: string | undefined): void {
  if (!isPaletteName(name)) {
    return;
  }
  state.activePalette = name;
  queryElements("[data-palette]", HTMLButtonElement).forEach(tab => tab.classList.toggle("active", tab.dataset.palette === name));
  renderConstructor();
}

queryElements("[data-tab]", HTMLButtonElement).forEach(button => {
  button.addEventListener("click", () => activateTab(button.dataset.tab || ""));
});

queryElements("[data-cook-mode]", HTMLButtonElement).forEach(button => {
  button.addEventListener("click", () => activateCookMode(button.dataset.cookMode));
});

queryElements("[data-palette]", HTMLButtonElement).forEach(button => {
  button.addEventListener("click", () => activatePalette(button.dataset.palette));
});

elements.loginForm.addEventListener("submit", event => {
  event.preventDefault();
  void submitLogin();
});

elements.logoutButton.addEventListener("click", () => runTask(handleLogout));

elements.refreshButton.addEventListener("click", () => runTask(refreshAll));
elements.webuiFilterInput.addEventListener("input", () => updateWebUIFilter(elements.webuiFilterInput.value));
elements.webuiGrid.addEventListener("click", event => {
  const target = elementTarget(event);
  const openID = target?.dataset.webuiOpen;
  if (openID) {
    openSelectedWebUI(openID);
    return;
  }
  const detailsID = target?.dataset.webuiDetails;
  if (detailsID) {
    showSelectedWebUIDialog(detailsID);
  }
});
elements.webuiGrid.addEventListener("change", event => {
  const target = elementTarget(event);
  const toggleID = target?.dataset.webuiToggle;
  if (toggleID && target instanceof HTMLInputElement) {
    runTask(() => setWebUIEnabled(toggleID, target.checked));
  }
});
elements.filterInput.addEventListener("input", renderTables);
elements.modelsTable.addEventListener("click", event => {
  const modelID = elementTarget(event)?.dataset.loadConfig;
  if (modelID) {
    runTask(() => loadSelectedConfig(modelID, refreshInventory));
  }
});
elements.benchmarkModelSelect.addEventListener("change", () => {
  selectBenchmarkModel(elements.benchmarkModelSelect.value);
  runTask(loadSelectedBenchmark);
});
elements.benchmarkTypeSelect.addEventListener("change", () => selectBenchmarkType(elements.benchmarkTypeSelect.value));
elements.benchmarkAllSections.addEventListener("change", () => toggleAllBenchmarkSections(elements.benchmarkAllSections.checked));
elements.benchmarkSections.addEventListener("change", updateBenchmarkSections);
elements.runBenchmarkButton.addEventListener("click", () => runTask(async () => {
  await runSelectedBenchmark();
  await refreshInventory();
}));
elements.analyticsPeriodSelect.addEventListener("change", () => runTask(async () => {
  updateAnalyticsPeriod(elements.analyticsPeriodSelect.value);
  await loadAnalytics();
}));
elements.analyticsNodeSelect.addEventListener("change", () => runTask(async () => {
  updateAnalyticsNode(elements.analyticsNodeSelect.value);
  await loadAnalytics();
}));
elements.analyticsModelSelect.addEventListener("change", () => runTask(async () => {
  updateAnalyticsModel(elements.analyticsModelSelect.value);
  await loadAnalytics();
}));
elements.analyticsSectionSelect.addEventListener("change", () => runTask(async () => {
  updateAnalyticsSection(elements.analyticsSectionSelect.value);
  await loadAnalytics();
}));
elements.analyticsRefreshButton.addEventListener("click", () => runTask(loadAnalytics));
elements.constructorFilterInput.addEventListener("input", renderConstructor);

elements.launchButton.addEventListener("click", () => runTask(handleLaunchRouter));

elements.restartButton.addEventListener("click", () => runTask(handleRestartRouter));

elements.shutdownButton.addEventListener("click", () => runTask(handleShutdownRouter));

elements.forceKillButton.addEventListener("click", () => runTask(handleForceKillRouter));

elements.previewButton.addEventListener("click", () => runTask(previewSimpleCook));
elements.cookForm.addEventListener("submit", event => {
  event.preventDefault();
  void applySimpleCook(refreshInventory);
});
elements.simpleNodeSelect.addEventListener("change", () => selectSimpleNode(elements.simpleNodeSelect.value));
elements.simpleConfigSelect.addEventListener("change", () => selectSimpleConfig(elements.simpleConfigSelect.value));
elements.simpleFieldFilter.addEventListener("input", () => updateSimpleFieldFilter(elements.simpleFieldFilter.value));
elements.simpleAddFieldButton.addEventListener("click", addSelectedSimpleField);
elements.simpleNewButton.addEventListener("click", newSimpleConfig);
elements.simpleCopyButton.addEventListener("click", copySimpleConfig);
elements.simpleDeleteButton.addEventListener("click", () => runTask(() => deleteSimpleConfig(refreshInventory)));
elements.simpleConfigEditor.addEventListener("change", event => updateSimpleField(event.target));
elements.simpleConfigEditor.addEventListener("toggle", event => updateSimpleSectionOpen(event.target), true);
elements.simpleConfigEditor.addEventListener("click", event => {
  const target = elementTarget(event);
  const fieldKey = target?.dataset.fieldValues;
  if (fieldKey) {
    showSimpleFieldValues(fieldKey, "field");
    return;
  }
  const modelFieldKey = target?.dataset.fieldModelValues;
  if (modelFieldKey) {
    showSimpleFieldValues(modelFieldKey, "model");
    return;
  }
  const removeKey = target?.dataset.removeSimpleField;
  if (removeKey) {
    removeSimpleField(removeKey);
  }
});
elements.simpleFieldSidebar.addEventListener("click", event => {
  const target = elementTarget(event);
  if (target?.dataset.closeFieldSidebar !== undefined) {
    state.simpleCook.sidebar = null;
    renderSimpleCook();
  }
});

elements.advancedPreviewButton.addEventListener("click", () => runTask(previewAdvancedCook));
elements.advancedApplyButton.addEventListener("click", () => runTask(() => applyAdvancedCook(refreshInventory)));
elements.clearConstructorButton.addEventListener("click", clearConstructor);
elements.advancedBackendSelect.addEventListener("change", () => updateConstructorBackendMode(elements.advancedBackendSelect.value));

elements.paletteList.addEventListener("dragstart", event => {
  if (!(event instanceof DragEvent)) {
    return;
  }
  const payloadID = closestElement(event.target, "[data-drag-payload]", HTMLElement)?.dataset.dragPayload;
  if (!payloadID || !event.dataTransfer) {
    return;
  }
  event.dataTransfer.setData("text/plain", payloadID);
  event.dataTransfer.effectAllowed = "copy";
});

elements.paletteList.addEventListener("click", event => {
  const target = elementTarget(event);
  const optionKey = target?.dataset.addOption;
  if (optionKey) {
    addOption(optionKey);
    return;
  }
  const payloadID = target?.dataset.selectPayload;
  if (payloadID) {
    addPayload(state.palettePayloads[payloadID]);
  }
});

elements.constructorLanes.addEventListener("dragover", event => {
  const drop = closestElement(event.target, "[data-drop-lane]", HTMLElement);
  if (!drop) {
    return;
  }
  event.preventDefault();
  drop.classList.add("drag-over");
});

elements.constructorLanes.addEventListener("dragleave", event => {
  closestElement(event.target, "[data-drop-lane]", HTMLElement)?.classList.remove("drag-over");
});

elements.constructorLanes.addEventListener("drop", event => {
  if (!(event instanceof DragEvent)) {
    return;
  }
  const drop = closestElement(event.target, "[data-drop-lane]", HTMLElement);
  if (!drop || !event.dataTransfer) {
    return;
  }
  event.preventDefault();
  drop.classList.remove("drag-over");
  addPayload(state.palettePayloads[event.dataTransfer.getData("text/plain")], drop.dataset.dropLane);
});

elements.constructorLanes.addEventListener("click", event => {
  const target = elementTarget(event);
  const clearLaneName = target?.dataset.clearLane;
  if (clearLaneName) {
    clearLane(clearLaneName);
    return;
  }
  const editLaneName = target?.dataset.editLaneFields;
  if (editLaneName) {
    editLaneFields(editLaneName);
  }
});
elements.constructorLanes.addEventListener("change", event => updateLaneTarget(event.target));

elements.constructorFieldDialog.addEventListener("cancel", event => {
  event.preventDefault();
  closeFieldEditor();
});
elements.constructorFieldDialog.addEventListener("click", event => {
  handleFieldEditorClick(event.target, renderConstructor);
});
elements.constructorFieldDialog.addEventListener("change", event => {
  handleFieldEditorInput(event.target);
});

elements.webuiDialog.addEventListener("cancel", event => {
  event.preventDefault();
  closeWebUIDialog();
});
elements.webuiDialog.addEventListener("click", event => {
  const target = elementTarget(event);
  if (target?.dataset.webuiDialogClose !== undefined) {
    closeWebUIDialog();
    return;
  }
  const enableID = target?.dataset.webuiEnable;
  if (enableID) {
    runTask(() => setWebUIEnabled(enableID, true));
    return;
  }
  const loadID = target?.dataset.webuiLoad;
  if (loadID) {
    runTask(() => loadSelectedWebUIModel(loadID, target.dataset.webuiLoadModel || "", target.dataset.webuiLoadImage || ""));
  }
});

elements.selectedOptionsList.addEventListener("input", event => updateOptionInput(event.target));
elements.selectedOptionsList.addEventListener("click", event => {
  const target = elementTarget(event);
  const removeKey = target?.dataset.removeOption;
  if (removeKey) {
    removeOption(removeKey);
    return;
  }
  const toggle = target?.dataset.toggleList;
  if (toggle) {
    toggleInspectorList(toggle);
  }
});

elements.usedModelsList.addEventListener("click", event => {
  const toggle = elementTarget(event)?.dataset.toggleList;
  if (toggle) {
    toggleInspectorList(toggle);
  }
});

elements.recipesList.addEventListener("click", event => {
  void handleRecipeClick(event);
});

void bootstrap();

async function submitLogin(): Promise<void> {
  elements.loginError.textContent = "";
  try {
    const session = await login(elements.tokenInput.value);
    state.csrf = session.csrf;
    showApp();
    await refreshAll();
  } catch (error) {
    elements.loginError.textContent = errorMessage(error);
  }
}

async function handleLogout(): Promise<void> {
  await logout();
  state.csrf = "";
  showLogin();
}

async function handleLaunchRouter(): Promise<void> {
  state.router = await launchRouter();
  renderRouterStatus();
  await loadWebUIs();
}

async function handleRestartRouter(): Promise<void> {
  state.router = await restartRouter();
  renderRouterStatus();
  await loadWebUIs();
}

async function handleShutdownRouter(): Promise<void> {
  state.router = await shutdownRouter();
  renderRouterStatus();
  await loadWebUIs();
}

async function handleForceKillRouter(): Promise<void> {
  state.router = await forceKillRouter();
  renderRouterStatus();
  await loadWebUIs();
}

async function handleRecipeClick(event: Event): Promise<void> {
  const id = elementTarget(event)?.dataset.deleteRecipe;
  if (!id) {
    return;
  }
  await deleteRecipe(id);
  await refreshInventory();
  renderRecipes();
}

function runTask(task: () => Promise<void>): void {
  void task();
}

function isCookMode(value: string | undefined): value is CookMode {
  return value === "quick" || value === "constructor";
}

function isPaletteName(value: string | undefined): value is PaletteName {
  return value === "configs" || value === "files" || value === "options";
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
