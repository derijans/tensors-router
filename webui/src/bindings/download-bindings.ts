import { confirmDestructive } from "../dialogs";
import { elementTarget } from "../dom";
import {
  bindDownloadCandidate,
  changeDownloadJob,
  chooseDownloadSearchResult,
  clearAllDownloadFilters,
  clearDownloadFilter,
  debounceDownloadSearch,
  loadDownloadLibrary,
  prefillDownloadContext,
  previewDownloadPlan,
  replaceDownloadCandidate,
  rescanDownloadLibrary,
  searchDownloadRepositories,
  selectDownloadFilterTab,
  selectDownloadNode,
  setPlannedDownloadSelection,
  startPlannedDownload,
  stopDownloadJobPolling,
  syncDownloadJobPolling,
  toggleDownloadFilter,
  toggleDownloadFilterGroup,
  togglePlannedDownloadFile,
  updateDownloadFilterSearch,
  updateDownloadSearchMode
} from "../downloads";
import { elements } from "../elements";
import { activateTab, onTabActivation } from "../shell/navigation";
import { state } from "../state";
import { runTask } from "../tasks";

interface ModelAssetHandoff {
  nodeID: string;
  publicID: string;
  configID: string;
  configFilename: string;
  field: string;
  position?: number;
  filename: string;
  hash: string;
}

export function bindDownloads(): void {
  onTabActivation(tab => {
    if (tab === "download") {
      syncDownloadJobPolling();
    } else {
      stopDownloadJobPolling();
    }
  });
  window.addEventListener("model-asset-handoff", event => {
    const detail = (event as CustomEvent<ModelAssetHandoff>).detail;
    if (detail) {
      prefillDownloadContext(detail);
      activateTab("download");
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
  elements.downloadPlanOutput.addEventListener("click", event => {
    const mode = elementTarget(event)?.dataset.downloadPlanSelect;
    if (mode === "all" || mode === "none" || mode === "required") {
      setPlannedDownloadSelection(mode);
    }
  });
  elements.downloadStartButton.addEventListener("click", () => runTask(startConfirmedDownload, "download-start", "download", "Starting download…"));
  elements.downloadRescanButton.addEventListener("click", () => runTask(rescanDownloadLibrary, "download-rescan", "download", "Scanning local library…"));
  elements.downloadSearchResults.addEventListener("click", handleSearchResultClick);
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
    const target = elementTarget(event);
    if (target?.dataset.downloadFilterClearAll !== undefined) {
      clearAllDownloadFilters();
      return;
    }
    const filter = target?.dataset.downloadFilterClear;
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
}

async function startConfirmedDownload(): Promise<void> {
  const unsafe = state.downloads.plan?.unsafe_warning || false;
  if (unsafe && !await confirmDestructive("Unsafe repository status", "Hugging Face reported an unsafe or pending security status. Download anyway?", "Download")) {
    return;
  }
  if (!await confirmDestructive("Start download?", "The selected node downloads directly from Hugging Face. Existing repository files are atomically replaced only after verification.", "Start")) {
    return;
  }
  await startPlannedDownload(unsafe, true);
}

function handleSearchResultClick(event: Event): void {
  const target = elementTarget(event);
  const repository = target?.closest<HTMLElement>("[data-download-repository]")?.dataset.downloadRepository;
  if (repository) {
    chooseDownloadSearchResult(repository);
    runTask(previewDownloadPlan, "download-plan", "download", "Preparing download plan…");
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
}
