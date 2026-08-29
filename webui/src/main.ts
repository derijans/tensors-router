import {
  deleteRecipe,
  forceKillRouter,
  fetchRoutingGroups,
  getInventory,
  getRouterStatus,
  getSession,
  launchRouter,
  login,
  logout,
  restartRouter,
  shutdownRouter,
  updateModelState
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
  changeDownloadJob,
  bindDownloadCandidate,
  chooseDownloadSearchResult,
  debounceDownloadSearch,
  loadDownloadLibrary,
  loadDownloads,
  previewDownloadPlan,
  prefillDownloadContext,
  replaceDownloadCandidate,
  rescanDownloadLibrary,
  searchDownloadRepositories,
  selectDownloadFilterTab,
  selectDownloadNode,
  startPlannedDownload,
  toggleDownloadFilter,
  toggleDownloadFilterGroup,
  togglePlannedDownloadFile,
  updateDownloadFilterSearch,
  updateDownloadSearchMode,
  clearDownloadFilter
} from "./downloads";
import {
  loadAnalytics,
  updateAnalyticsModel,
  updateAnalyticsNode,
  updateAnalyticsPeriod,
  updateAnalyticsSection
} from "./analytics";
import {
  loadLoadCaptures,
  loadMoreCaptureOutput,
  selectLoadCapture,
  updateLoadCaptureFilters
} from "./load-captures";
import { closestElement, elementTarget, queryElements } from "./dom";
import { bootstrapApplication } from "./bootstrap";
import { elements } from "./elements";
import { state } from "./state";
import { confirmDestructive, registerSafetyDialog } from "./dialogs";
import { openRoutingGroupDialog, registerRoutingGroupDialog } from "./routing-groups-dialog";
import { confirmDiscardDirtyWork, markConstructorClean, markSimpleCookClean, registerDirtyStateGuard } from "./dirty-state";
import { registerOperationRetry, runOperation } from "./operations";
import { clearConversionWarnings, invalidateAcceptedConversions } from "./conversions";
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
import { loadSelectedConfig, resolveFilteredModels, retryModelAssetResolution } from "./model-actions";
import { calculateModelFileHash, copyModelFileHash } from "./model-files";
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
  importSimpleConfig,
  newSimpleConfig,
  previewSimpleCook,
  removeSimpleField,
  renderSimpleCook,
  selectSimpleConfig,
  selectSimpleNode,
  showSimpleFieldValues,
  updateSimpleField,
  updateSimpleFieldFilter,
  updateSimpleSectionOpen,
  exportSimpleConfig
} from "./simple-cook";
import {
  renderInventory,
  renderRecipes,
  renderRouterStatus,
  renderTables,
  showApp,
  showLogin
} from "./render-dashboard";
import { activateModelInventorySubtab, updateConfigNodeFilter, updateFileNodeFilter } from "./model-inventory";
import type { CookMode, PaletteName } from "./types";
import { persistModelEnabled } from "./model-state-action";
import { handleNodesClick, setNodesTabActive, stopNodeStatePolling } from "./nodes-state";

async function bootstrap(): Promise<void> {
  await bootstrapApplication({
    getSession,
    applySession: session => { state.csrf = session.csrf; },
    showApp,
    showLogin,
    loadInitialData: async () => {
      await runOperation({
        key: "initial-load",
        group: "refresh",
        label: "Loading data…",
        task: refreshAll
      });
    }
  });
}

async function refreshAll(): Promise<void> {
  await refreshRouterStatus();
  await refreshInventory(state.activeTab === "models");
  await loadWebUIs();
  await loadAnalytics();
  await loadDownloads();
  await loadLoadCaptures();
}

async function refreshRouterStatus(): Promise<void> {
  state.router = await getRouterStatus();
  renderRouterStatus();
}

async function refreshInventory(includeFiles = state.activeTab === "models"): Promise<void> {
  state.inventory = await getInventory(includeFiles);
  // Routing groups decide what the models table shows in its routing column, so
  // they are refreshed alongside it. A router with none configured reports an
  // empty list, which leaves every row showing no peers.
  state.routingGroups = await fetchRoutingGroups().catch(() => null);
  renderInventory();
}

function activateTab(name: string): void {
  state.activeTab = name;
  queryElements("[data-tab]", HTMLButtonElement).forEach(tab => tab.classList.toggle("active", tab.dataset.tab === name));
  queryElements("[data-panel]", HTMLElement).forEach(panel => panel.classList.toggle("active", panel.dataset.panel === name));
  setNodesTabActive(name === "nodes");
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
  button.addEventListener("click", () => {
    const tab = button.dataset.tab || "";
    activateTab(tab);
    if (tab === "models") {
      runTask(() => refreshInventory(true), "models-inventory", "models", "Scanning model files…");
    }
  });
});

window.addEventListener("model-asset-handoff", event => {
  const detail = (event as CustomEvent<{nodeID: string; publicID: string; configID: string; configFilename: string; field: string; position?: number; filename: string; hash: string}>).detail;
  if (!detail) {
    return;
  }
  prefillDownloadContext(detail);
  activateTab("download");
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

elements.logoutButton.addEventListener("click", () => runTask(async () => {
  if (await confirmDiscardDirtyWork("Logging out")) {
    await handleLogout();
  }
}, "logout", "session", "Logging out…"));

elements.refreshButton.addEventListener("click", () => runTask(refreshAll, "refresh-all", "refresh", "Refreshing data…"));
elements.nodesGrid.addEventListener("click", handleNodesClick);
elements.nodesRefreshButton.addEventListener("click", () => runTask(() => refreshInventory(false), "nodes-refresh", "nodes", "Refreshing nodes…"));
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
    runTask(() => setWebUIEnabled(toggleID, target.checked), `webui-toggle-${toggleID}`, "webui", "Updating backend UI…");
  }
});
elements.downloadNodeSelect.addEventListener("change", () => runTask(async () => {
  selectDownloadNode(elements.downloadNodeSelect.value);
  await loadDownloadLibrary();
}, "download-node", "download", "Changing download node…"));
elements.downloadSearchButton.addEventListener("click", () => runTask(searchDownloadRepositories, "download-search", "download", "Searching Hugging Face…"));
elements.downloadSearchInput.addEventListener("input", debounceDownloadSearch);
elements.downloadSearchMode.addEventListener("change", updateDownloadSearchMode);
elements.downloadFilterSearch.addEventListener("input", updateDownloadFilterSearch);
elements.downloadNextPageButton.addEventListener("click", () => runTask(() => searchDownloadRepositories(true), "download-search-next", "download", "Loading more models…"));
elements.downloadPlanButton.addEventListener("click", () => runTask(previewDownloadPlan, "download-plan", "download", "Preparing download plan…"));
elements.downloadPlanOutput.addEventListener("change", event => {
  const path = elementTarget(event)?.dataset.downloadPlanFile;
  if (path) {
    togglePlannedDownloadFile(path);
  }
});
elements.downloadStartButton.addEventListener("click", () => runTask(async () => {
  const unsafe = state.downloads.plan?.unsafe_warning || false;
  if (unsafe && !await confirmDestructive("Unsafe repository status", "Hugging Face reported an unsafe or pending security status. Download anyway?", "Download")) {
    return;
  }
  if (!await confirmDestructive("Start download?", "The selected node downloads directly from Hugging Face. Existing repository files are atomically replaced only after verification.", "Start")) {
    return;
  }
  await startPlannedDownload(unsafe, true);
}, "download-start", "download", "Starting download…"));
elements.downloadRescanButton.addEventListener("click", () => runTask(rescanDownloadLibrary, "download-rescan", "download", "Scanning local library…"));
elements.downloadSearchResults.addEventListener("click", event => {
  const target = elementTarget(event);
  const repository = target?.dataset.downloadRepository;
  if (repository) {
    chooseDownloadSearchResult(repository);
  }
  const candidateIndex = target?.dataset.downloadCandidateBind;
  if (candidateIndex !== undefined) {
    runTask(() => bindDownloadCandidate(Number(candidateIndex)), `download-bind-${candidateIndex}`, "download", "Binding verified origin…");
  }
  const replacementIndex = target?.dataset.downloadCandidateReplace;
  if (replacementIndex !== undefined) {
    void confirmDestructive(
      "Replace expected model asset?",
      "The selected Hugging Face file has a different SHA-256. This intentionally changes the saved config to a different model.",
      "Replace model"
    ).then(confirmed => {
      if (confirmed) {
        runTask(() => replaceDownloadCandidate(Number(replacementIndex)), `download-replace-${replacementIndex}`, "download", "Replacing expected model…");
      }
    });
  }
});
elements.downloadFilterTabs.addEventListener("click", event => {
  const tab = elementTarget(event)?.dataset.downloadFilterTab;
  if (tab) {
    selectDownloadFilterTab(tab);
  }
});
elements.downloadFilterOptions.addEventListener("click", event => {
  const target = elementTarget(event);
  const filter = target?.dataset.downloadFilter;
  if (filter) {
    toggleDownloadFilter(filter);
  }
  const group = target?.dataset.downloadFilterGroup;
  if (group) {
    toggleDownloadFilterGroup(group);
  }
});
elements.downloadFilterSummary.addEventListener("click", event => {
  const filter = elementTarget(event)?.dataset.downloadFilterClear;
  if (filter) {
    clearDownloadFilter(filter);
  }
});
elements.downloadJobs.addEventListener("click", event => {
  const target = elementTarget(event);
  const jobID = target?.dataset.downloadJob;
  const action = target?.dataset.downloadAction;
  if (jobID && (action === "pause" || action === "resume" || action === "cancel")) {
    runTask(() => changeDownloadJob(jobID, action), `download-${action}-${jobID}`, "download", `${action} download…`);
  }
});
elements.modelInventorySubtabs.addEventListener("click", event => {
  const subtab = elementTarget(event)?.dataset.modelInventorySubtab;
  if (subtab === "models" || subtab === "files") {
    activateModelInventorySubtab(subtab);
  }
});
elements.modelSearchInput.addEventListener("input", () => {
  state.models.modelSearch = elements.modelSearchInput.value;
  renderTables();
});
elements.modelEnabledFilter.addEventListener("change", () => {
  state.models.enabledFilter = elements.modelEnabledFilter.value;
  renderTables();
});
elements.modelBackendFilter.addEventListener("change", () => {
  state.models.backendFilter = elements.modelBackendFilter.value;
  renderTables();
});
elements.modelCapabilityFilter.addEventListener("change", () => {
  state.models.capabilityFilter = elements.modelCapabilityFilter.value;
  renderTables();
});
elements.fileSearchInput.addEventListener("input", () => {
  state.models.fileSearch = elements.fileSearchInput.value;
  renderTables();
});
elements.fileRoleFilter.addEventListener("change", () => {
  state.models.fileRoleFilter = elements.fileRoleFilter.value;
  renderTables();
});
elements.fileExtensionFilter.addEventListener("change", () => {
  state.models.fileExtensionFilter = elements.fileExtensionFilter.value;
  renderTables();
});
elements.fileHashFilter.addEventListener("change", () => {
  state.models.fileHashFilter = elements.fileHashFilter.value;
  renderTables();
});
elements.modelsNodeFilter.addEventListener("change", () => updateConfigNodeFilter([...elements.modelsNodeFilter.selectedOptions].map(option => option.value)));
elements.filesNodeFilter.addEventListener("change", () => updateFileNodeFilter([...elements.filesNodeFilter.selectedOptions].map(option => option.value)));
elements.resolveFilteredModelsButton.addEventListener("click", () => runTask(() => resolveFilteredModels(refreshInventory), "resolve-filtered-models", "models", "Resolving visible configs…"));
elements.modelsActionStatus.addEventListener("click", event => {
  const key = elementTarget(event)?.dataset.modelResolutionRetry;
  if (key) {
    runTask(() => retryModelAssetResolution(key, refreshInventory), `resolve-retry-${key}`, "models", "Retrying resolution…");
  }
});
elements.modelsTable.addEventListener("click", event => {
  const target = elementTarget(event);
  const modelID = target?.dataset.loadConfig;
  if (modelID) {
    runTask(() => loadSelectedConfig(modelID, refreshInventory), `model-load-${modelID}`, "webui", "Loading model…");
    return;
  }
  const routingNode = target?.dataset.routingNode;
  const routingImage = target?.dataset.routingImage;
  if (routingNode && routingImage) {
    runTask(
      () => openRoutingGroupDialog({node_id: routingNode, image_id: routingImage}),
      `routing-group-${routingNode}-${routingImage}`,
      "models",
      "Loading routing group"
    );
  }
});
elements.modelsTable.addEventListener("change", event => {
  const target = elementTarget(event);
  if (!(target instanceof HTMLInputElement)) {
    return;
  }
  const nodeID = target.dataset.modelEnabledNode;
  const localID = target.dataset.modelEnabledId;
  if (!nodeID || !localID) {
    return;
  }
  const enabled = target.checked;
  runTask(async () => {
    await persistModelEnabled(
      {node_id: nodeID, local_id: localID, enabled},
      updateModelState,
      () => refreshInventory(true),
      () => { target.checked = !enabled; }
    );
  }, `model-state-${nodeID}-${localID}`, `model-state-${nodeID}-${localID}`, `${enabled ? "Enabling" : "Disabling"} ${localID}…`);
});
elements.filesTable.addEventListener("click", event => {
  const target = elementTarget(event);
  const nodeID = target?.dataset.hashFileNode;
  const path = target?.dataset.hashFilePath;
  if (nodeID && path) {
    runTask(() => calculateModelFileHash(nodeID, path), `hash-file-${encodeURIComponent(nodeID)}-${encodeURIComponent(path)}`, "models", "Hashing model file…");
    return;
  }
  const hash = target?.dataset.copyFileHash;
  if (hash) {
    runTask(() => copyModelFileHash(hash), `copy-file-hash-${hash}`, "models-copy", "Copying SHA-256…");
  }
});
elements.benchmarkModelSelect.addEventListener("change", () => {
  selectBenchmarkModel(elements.benchmarkModelSelect.value);
  runTask(loadSelectedBenchmark, "benchmark-load", "benchmark", "Loading benchmark…");
});
elements.benchmarkTypeSelect.addEventListener("change", () => selectBenchmarkType(elements.benchmarkTypeSelect.value));
elements.benchmarkAllSections.addEventListener("change", () => toggleAllBenchmarkSections(elements.benchmarkAllSections.checked));
elements.benchmarkSections.addEventListener("change", updateBenchmarkSections);
elements.runBenchmarkButton.addEventListener("click", () => runTask(async () => {
  await runSelectedBenchmark();
  await refreshInventory();
}, "benchmark-run", "benchmark", "Running benchmark…"));
elements.analyticsPeriodSelect.addEventListener("change", () => runTask(async () => {
  updateAnalyticsPeriod(elements.analyticsPeriodSelect.value);
  await loadAnalytics();
}, "analytics-period", "analytics", "Loading analytics…"));
elements.analyticsNodeSelect.addEventListener("change", () => runTask(async () => {
  updateAnalyticsNode(elements.analyticsNodeSelect.value);
  await loadAnalytics();
}, "analytics-node", "analytics", "Loading analytics…"));
elements.analyticsModelSelect.addEventListener("change", () => runTask(async () => {
  updateAnalyticsModel(elements.analyticsModelSelect.value);
  await loadAnalytics();
}, "analytics-model", "analytics", "Loading analytics…"));
elements.analyticsSectionSelect.addEventListener("change", () => runTask(async () => {
  updateAnalyticsSection(elements.analyticsSectionSelect.value);
  await loadAnalytics();
}, "analytics-section", "analytics", "Loading analytics…"));
elements.analyticsRefreshButton.addEventListener("click", () => runTask(loadAnalytics, "analytics-refresh", "analytics", "Loading analytics…"));
elements.loadCaptureRefreshButton.addEventListener("click", () => runTask(async () => {
  updateLoadCaptureFilters();
  await loadLoadCaptures();
}, "load-captures-refresh", "load-captures", "Loading captures…"));
elements.loadCaptureMoreButton.addEventListener("click", () => runTask(() => loadLoadCaptures(false), "load-captures-more", "load-captures", "Loading captures…"));
elements.loadCaptureOutputMoreButton.addEventListener("click", () => runTask(loadMoreCaptureOutput, "load-capture-output", "load-captures", "Loading output…"));
elements.loadCaptureRows.addEventListener("click", event => {
  const target = elementTarget(event);
  const nodeID = target?.dataset.loadCaptureNode;
  const attemptID = target?.dataset.loadCaptureId;
  if (nodeID && attemptID) {
    runTask(() => selectLoadCapture(nodeID, attemptID), `load-capture-${attemptID}`, "load-captures", "Loading capture…");
  }
});

elements.constructorFilterInput.addEventListener("input", renderConstructor);

elements.launchButton.addEventListener("click", () => runTask(handleLaunchRouter, "router-launch", "router", "Launching router…"));

elements.restartButton.addEventListener("click", () => runTask(async () => {
  if (await confirmDestructive("Restart router?", "Active requests may be interrupted if they cannot finish during the drain period.", "Restart")) {
    await handleRestartRouter();
  }
}, "router-restart", "router", "Restarting router…"));

elements.shutdownButton.addEventListener("click", () => runTask(async () => {
  if (await confirmDestructive("Shutdown router?", "The router will stop accepting new work and drain active transfers.", "Shutdown")) {
    await handleShutdownRouter();
  }
}, "router-shutdown", "router", "Shutting down router…"));

elements.forceKillButton.addEventListener("click", () => runTask(async () => {
  if (await confirmDestructive("Force-kill router?", "Active requests will be terminated immediately and may fail.", "Force kill")) {
    await handleForceKillRouter();
  }
}, "router-force-kill", "router", "Force-killing router…"));

elements.previewButton.addEventListener("click", () => runTask(previewSimpleCook, "quick-preview", "cook", "Preparing preview…"));
elements.simpleExportButton.addEventListener("click", () => runTask(exportSimpleConfig, "quick-export", "cook", "Exporting KCPPS…"));
elements.simpleImportButton.addEventListener("click", () => runTask(async () => {
  if (await confirmDiscardDirtyWork("Importing a configuration")) {
    elements.simpleImportInput.click();
  }
}, "quick-import-pick", "cook-selection", "Choosing file…"));
elements.simpleImportInput.addEventListener("change", () => runTask(async () => {
  const file = elements.simpleImportInput.files?.[0];
  elements.simpleImportInput.value = "";
  if (file) {
    await importSimpleConfig(file);
  }
}, "quick-import", "cook", "Importing KCPPS…"));
elements.cookForm.addEventListener("submit", event => {
  event.preventDefault();
  runTask(() => applySimpleCook(refreshInventory), "quick-apply", "cook", "Applying config…");
});
elements.simpleNodeSelect.addEventListener("change", () => runTask(async () => {
  if (await confirmDiscardDirtyWork("Changing nodes")) {
    selectSimpleNode(elements.simpleNodeSelect.value);
  } else {
    renderSimpleCook();
  }
}, "quick-node-change", "cook-selection", "Changing node…"));
elements.simpleConfigSelect.addEventListener("change", () => runTask(async () => {
  if (await confirmDiscardDirtyWork("Changing configurations")) {
    selectSimpleConfig(elements.simpleConfigSelect.value);
  } else {
    renderSimpleCook();
  }
}, "quick-config-change", "cook-selection", "Changing config…"));
elements.simpleFieldFilter.addEventListener("input", () => updateSimpleFieldFilter(elements.simpleFieldFilter.value));
elements.cookIdInput.addEventListener("input", invalidateAcceptedConversions);
elements.advancedCookIdInput.addEventListener("input", invalidateAcceptedConversions);
elements.simpleAddFieldButton.addEventListener("click", addSelectedSimpleField);
elements.simpleNewButton.addEventListener("click", () => runTask(async () => {
  if (await confirmDiscardDirtyWork("Creating a new configuration")) {
    newSimpleConfig();
  }
}, "quick-new", "cook-selection", "Opening new config…"));
elements.simpleCopyButton.addEventListener("click", () => runTask(async () => {
  if (await confirmDiscardDirtyWork("Copying this configuration")) {
    copySimpleConfig();
  }
}, "quick-copy", "cook-selection", "Copying config…"));
elements.simpleDeleteButton.addEventListener("click", () => runTask(() => deleteSimpleConfig(refreshInventory), "quick-delete", "cook", "Deleting config…"));
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

elements.advancedPreviewButton.addEventListener("click", () => runTask(previewAdvancedCook, "constructor-preview", "cook", "Preparing preview…"));
elements.advancedApplyButton.addEventListener("click", () => runTask(() => applyAdvancedCook(refreshInventory), "constructor-apply", "cook", "Applying cook plan…"));
elements.clearConstructorButton.addEventListener("click", () => runTask(async () => {
  if (await confirmDestructive("Clear constructor?", "All selected lanes and option changes will be discarded.", "Clear")) {
    clearConstructor();
    markConstructorClean();
  }
}, "constructor-clear", "cook-selection", "Clearing constructor…"));
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
    runTask(async () => {
      if (await confirmDestructive("Clear lane?", `The ${clearLaneName} selection and its overrides will be discarded.`, "Clear lane")) {
        clearLane(clearLaneName);
      }
    }, `lane-clear-${clearLaneName}`, "cook-selection", "Clearing lane…");
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
    runTask(() => setWebUIEnabled(enableID, true), `webui-enable-${enableID}`, "webui", "Enabling backend UI…");
    return;
  }
  const loadID = target?.dataset.webuiLoad;
  if (loadID) {
    runTask(() => loadSelectedWebUIModel(loadID, target.dataset.webuiLoadModel || "", target.dataset.webuiLoadImage || ""), `webui-load-${loadID}`, "webui", "Loading backend UI model…");
  }
});

elements.selectedOptionsList.addEventListener("input", event => updateOptionInput(event.target));
elements.selectedOptionsList.addEventListener("change", event => updateOptionInput(event.target));
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
  runTask(() => handleRecipeClick(event), "recipe-delete", "cook", "Deleting recipe…");
});

registerSafetyDialog();
registerRoutingGroupDialog(refreshInventory);
registerOperationRetry();
registerDirtyStateGuard();
markConstructorClean();
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
  stopNodeStatePolling();
  state.csrf = "";
  markSimpleCookClean();
  markConstructorClean();
  clearConversionWarnings();
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
  if (!await confirmDestructive("Delete recipe?", `Delete ${id}? This removes the public split route.`, "Delete")) {
    return;
  }
  await deleteRecipe(id);
  await refreshInventory();
  renderRecipes();
}

function runTask(task: () => Promise<void>, key = "general", group = "general", label = "Working…"): void {
  void runOperation({key, group, label, task}).catch(() => undefined);
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
