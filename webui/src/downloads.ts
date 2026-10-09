import { emptyHTML, html, listOrFallback, setHTML } from "./safe-html";
import { optionElement } from "./markup-primitives";
import {
  createDownloadJob,
  downloadJobAction,
  getDownloadCapabilities,
  getDownloadLibrary,
  planDownload,
  rescanDownloads
} from "./api";
import { elements } from "./elements";
import { downloadNodeStatus, enabledDownloadNodes, preferredDownloadNodeID } from "./download-capability-data";
import { planSelectionForMode, selectedDownloadFiles, toggleDownloadPath } from "./download-plan-data";
import { trackDownloadProgress, type DownloadProgressTracking } from "./download-job-data";
import { renderDownloadJob } from "./download-job-view";
import { renderDownloadLibrary } from "./download-library-view";
import { renderDownloadFilters, renderPlan, renderSearchResults } from "./download-search-view";
import { downloadToken, requestedFiles } from "./download-inputs";
import { state } from "./state";
import type { DownloadLibraryResponse } from "./types";
import { setControlUnavailable } from "./operations";

export async function loadDownloads(): Promise<void> {
  try {
    const capabilities = await getDownloadCapabilities();
    state.downloads.capabilities = capabilities;
    const enabledNodes = enabledDownloadNodes(capabilities.nodes);
    state.downloads.available = enabledNodes.length > 0;
    state.downloads.nodeID = preferredDownloadNodeID(capabilities.nodes, state.downloads.nodeID);
    const selectedNode = enabledNodes.find(node => node.node_id === state.downloads.nodeID);
    state.downloads.error = "";
    if (selectedNode?.capability.working) {
      try {
        acceptDownloadLibrary(await getDownloadLibrary(state.downloads.nodeID));
      } catch (error) {
        state.downloads.library = null;
        state.downloads.error = error instanceof Error ? error.message : String(error);
      }
    } else {
      state.downloads.library = null;
    }
  } catch (error) {
    state.downloads.available = false;
    state.downloads.error = error instanceof Error ? error.message : String(error);
  }
  renderDownloads();
}

export function selectDownloadNode(nodeID: string): void {
  state.downloads.nodeID = nodeID;
  state.downloads.plan = null;
  state.downloads.library = null;
}

export async function loadDownloadLibrary(): Promise<void> {
  const selectedNode = state.downloads.capabilities?.nodes.find(node => node.node_id === state.downloads.nodeID);
  if (!selectedNode?.capability.working) {
    state.downloads.library = null;
    renderDownloads();
    return;
  }
  acceptDownloadLibrary(await getDownloadLibrary(state.downloads.nodeID));
  renderDownloads();
  syncDownloadJobPolling();
}

let progressTracking: DownloadProgressTracking = {samples: new Map(), bytesPerSecond: new Map()};

function acceptDownloadLibrary(library: DownloadLibraryResponse): void {
  state.downloads.library = library;
  state.downloads.pollError = "";
  progressTracking = trackDownloadProgress(progressTracking, library.jobs, Date.now());
}

const jobProgressIntervalMilliseconds = 1500;
let jobPollTimer: number | undefined;
let jobPollNodeID = "";

function hasActiveDownloadJob(): boolean {
  return (state.downloads.library?.jobs || []).some(job => job.state === "queued" || job.state === "running");
}

export function syncDownloadJobPolling(): void {
  const shouldPoll = state.activeTab === "download" && state.downloads.available && hasActiveDownloadJob();
  if (!shouldPoll) {
    stopDownloadJobPolling();
    return;
  }
  jobPollNodeID = state.downloads.nodeID;
  if (jobPollTimer !== undefined) {
    return;
  }
  jobPollTimer = window.setTimeout(() => void runDownloadJobPoll(), jobProgressIntervalMilliseconds);
}

export function stopDownloadJobPolling(): void {
  if (jobPollTimer !== undefined) {
    window.clearTimeout(jobPollTimer);
    jobPollTimer = undefined;
  }
}

async function runDownloadJobPoll(): Promise<void> {
  jobPollTimer = undefined;
  if (state.activeTab !== "download" || state.downloads.nodeID !== jobPollNodeID || !hasActiveDownloadJob()) {
    return;
  }
  try {
    acceptDownloadLibrary(await getDownloadLibrary(state.downloads.nodeID));
  } catch (error) {
    state.downloads.pollError = error instanceof Error ? error.message : String(error);
  }
  renderDownloads();
  syncDownloadJobPolling();
}

export function setPlannedDownloadSelection(mode: "all" | "none" | "required"): void {
  const plan = state.downloads.plan;
  if (!plan) {
    return;
  }
  state.downloads.selectedPlanFiles = planSelectionForMode(plan, mode);
  renderDownloads();
}

export async function previewDownloadPlan(): Promise<void> {
  const repository = elements.downloadRepositoryInput.value.trim();
  if (!state.downloads.nodeID || !repository) {
    throw new Error("Select a node and enter an owner/repository")
  }
  const revision = elements.downloadRevisionInput.value.trim();
  const token = downloadToken();
  state.downloads.plan = await planDownload({
    node_id: state.downloads.nodeID,
    repository,
    ...(revision ? {revision} : {}),
    files: requestedFiles(),
    mode: "smart",
    ...(token ? {token} : {})
  });
  state.downloads.selectedPlanFiles = state.downloads.plan.files.map(file => file.path);
  state.downloads.error = "";
  renderDownloads();
}

export async function startPlannedDownload(confirmUnsafe: boolean, confirmReplace: boolean): Promise<void> {
  const plan = state.downloads.plan;
  if (!plan || !state.downloads.nodeID) {
    throw new Error("Preview a download plan first")
  }
  const files = selectedDownloadFiles(plan, state.downloads.selectedPlanFiles);
  if (files.length === 0) {
    throw new Error("Select at least one file to download")
  }
  const token = downloadToken();
  await createDownloadJob({
    node_id: state.downloads.nodeID,
    repository: plan.repository,
    revision: plan.commit || plan.revision,
    files: files.map(file => file.path),
    mode: "explicit",
    ...(token ? {token} : {}),
    confirm_unsafe: confirmUnsafe,
    confirm_replace: confirmReplace
  });
  state.downloads.plan = null;
  state.downloads.selectedPlanFiles = [];
  elements.downloadTokenInput.value = "";
  await loadDownloadLibrary();
}

export async function rescanDownloadLibrary(): Promise<void> {
  if (!state.downloads.nodeID) {
    throw new Error("Select a download node first")
  }
  await rescanDownloads(state.downloads.nodeID);
  await loadDownloadLibrary();
}

export async function changeDownloadJob(jobID: string, action: "pause" | "resume" | "cancel"): Promise<void> {
  if (!state.downloads.nodeID) {
    throw new Error("Select a download node first")
  }
  await downloadJobAction(state.downloads.nodeID, jobID, action);
  await loadDownloadLibrary();
}

export function chooseDownloadSearchResult(repository: string): void {
  elements.downloadRepositoryInput.value = repository;
  elements.downloadRevisionInput.value = "";
  elements.downloadFilesInput.value = "";
  state.downloads.selectedRepository = repository;
  renderDownloads();
}

export function togglePlannedDownloadFile(path: string): void {
  const plan = state.downloads.plan;
  if (!plan?.files.some(file => file.path === path)) {
    return;
  }
  state.downloads.selectedPlanFiles = toggleDownloadPath(state.downloads.selectedPlanFiles, path);
  renderDownloads();
}

export function renderDownloads(): void {
  const available = state.downloads.available;
  elements.downloadFlag.hidden = !hasActiveDownloadJob();
  elements.downloadTab.hidden = false;
  elements.downloadPanel.hidden = false;
  if (!available) {
    elements.downloadStatus.textContent = state.downloads.error || "No node on this cluster has the downloader enabled.";
    setHTML(elements.downloadSearchResults, emptyHTML);
    elements.downloadNextPageButton.hidden = true;
    setHTML(elements.downloadPlanOutput, emptyHTML);
    setHTML(elements.downloadJobs, emptyHTML);
    setHTML(elements.downloadLibrary, emptyHTML);
    setDownloadControlsWorking(false);
    setControlUnavailable(elements.downloadStartButton, true);
    return;
  }
  const nodes = enabledDownloadNodes(state.downloads.capabilities?.nodes || []);
  setHTML(elements.downloadNodeSelect, html`${nodes.map(node => optionElement(node.node_id, downloadNodeLabel(node), node.node_id === state.downloads.nodeID))}`);


  const node = nodes.find(value => value.node_id === state.downloads.nodeID);
  const working = node?.capability.working === true;
  const configuredToken = node?.capability.configured_token ? "configured fallback token is available" : "anonymous access unless a temporary token is entered";
  elements.downloadStatus.textContent = state.downloads.error || (node && !working ? downloadNodeStatus(node) : configuredToken);
  renderDownloadFilters();
  setHTML(elements.downloadSearchResults, renderSearchResults());
  elements.downloadNextPageButton.hidden = !state.downloads.nextCursor || state.downloads.searchStatus === "searching";
  setHTML(elements.downloadPlanOutput, state.downloads.plan ? renderPlan(state.downloads.plan) : emptyHTML);
  const staleNotice = state.downloads.pollError ? html`<p class="error-text">Progress may be stale: ${state.downloads.pollError}</p>` : emptyHTML;
  const jobs = listOrFallback((state.downloads.library?.jobs || []).map(job => renderDownloadJob(job, progressTracking.bytesPerSecond.get(job.id))), html`<p class="muted">No download jobs on this node.</p>`);
  setHTML(elements.downloadJobs, html`${staleNotice}${jobs}`);
  setHTML(elements.downloadLibrary, renderDownloadLibrary(state.downloads.library));
  setDownloadControlsWorking(working);
  setControlUnavailable(elements.downloadStartButton, !working || state.downloads.plan === null || state.downloads.selectedPlanFiles.length === 0);
  syncDownloadJobPolling();
}

function setDownloadControlsWorking(working: boolean): void {
  elements.downloadPanel.querySelectorAll<HTMLButtonElement | HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>("button, input, select, textarea").forEach(control => {
    if (control !== elements.downloadNodeSelect) {
      setControlUnavailable(control, !working);
    }
  });
}

function downloadNodeLabel(node: Parameters<typeof downloadNodeStatus>[0]): string {
  return `${node.node_id} — ${downloadNodeStatus(node)}`;
}
