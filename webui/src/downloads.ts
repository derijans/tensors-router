import {
  createDownloadJob,
  downloadJobAction,
  getDownloadCapabilities,
  getDownloadLibrary,
  planDownload,
  rescanDownloads,
  searchDownloads
} from "./api";
import { elements } from "./elements";
import { state } from "./state";
import { escapeAttribute, escapeHTML, formatBytes } from "./utils";
import type { DownloadJob } from "./types";

export async function loadDownloads(): Promise<void> {
  try {
    const capabilities = await getDownloadCapabilities();
    state.downloads.capabilities = capabilities;
    state.downloads.available = capabilities.available !== false && capabilities.nodes.length > 0;
    const selected = state.downloads.nodeID;
    if (!capabilities.nodes.some(node => node.node_id === selected)) {
      state.downloads.nodeID = capabilities.nodes[0]?.node_id || "";
    }
    if (state.downloads.available && state.downloads.nodeID) {
      state.downloads.library = await getDownloadLibrary(state.downloads.nodeID);
    } else {
      state.downloads.library = null;
    }
    state.downloads.error = "";
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
  if (!state.downloads.nodeID) {
    return;
  }
  state.downloads.library = await getDownloadLibrary(state.downloads.nodeID);
  renderDownloads();
}

export async function searchDownloadRepositories(): Promise<void> {
  if (!state.downloads.nodeID) {
    throw new Error("Select a download node first");
  }
  const token = downloadToken();
  state.downloads.search = await searchDownloads({
    node_id: state.downloads.nodeID,
    query: elements.downloadSearchInput.value.trim(),
    ...(token ? {token} : {})
  });
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
  state.downloads.error = "";
  renderDownloads();
}

export async function startPlannedDownload(confirmUnsafe: boolean, confirmReplace: boolean): Promise<void> {
  const plan = state.downloads.plan;
  if (!plan || !state.downloads.nodeID) {
    throw new Error("Preview a download plan first")
  }
  const token = downloadToken();
  await createDownloadJob({
    node_id: state.downloads.nodeID,
    repository: plan.repository,
    revision: plan.revision,
    files: plan.files.map(file => file.path),
    ...(token ? {token} : {}),
    confirm_unsafe: confirmUnsafe,
    confirm_replace: confirmReplace
  });
  state.downloads.plan = null;
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
  elements.downloadSearchResults.innerHTML = "";
}

export function renderDownloads(): void {
  elements.downloadTab.hidden = !state.downloads.available;
  elements.downloadPanel.hidden = !state.downloads.available;
  if (!state.downloads.available) {
    return;
  }
  const nodes = state.downloads.capabilities?.nodes || [];
  elements.downloadNodeSelect.innerHTML = nodes.map(node => {
    const enabled = node.available && node.capability.configured;
    const status = enabled ? "ready" : node.capability.error || "downloader unavailable";
    return `<option value="${escapeAttribute(node.node_id)}"${node.node_id === state.downloads.nodeID ? " selected" : ""}${enabled ? "" : " disabled"}>${escapeHTML(node.node_id)} — ${escapeHTML(status)}</option>`;
  }).join("");
  const node = nodes.find(value => value.node_id === state.downloads.nodeID);
  const configuredToken = node?.capability.configured_token ? "configured fallback token is available" : "anonymous access unless a temporary token is entered";
  elements.downloadStatus.textContent = state.downloads.error || configuredToken;
  elements.downloadStartButton.disabled = state.downloads.plan === null;
  elements.downloadSearchResults.innerHTML = state.downloads.search.map(result => `
    <button class="download-entry" type="button" data-download-repository="${escapeAttribute(result.id)}">
      <strong>${escapeHTML(result.id)}</strong><span>${result.downloads} downloads · ${result.likes} likes${result.gated ? " · gated" : ""}</span>
    </button>
  `).join("");
  elements.downloadPlanOutput.innerHTML = state.downloads.plan ? renderPlan(state.downloads.plan) : "";
  elements.downloadJobs.innerHTML = (state.downloads.library?.jobs || []).map(renderJob).join("") || "<p class=\"muted\">No download jobs on this node.</p>";
  elements.downloadLibrary.innerHTML = (state.downloads.library?.artifacts || []).map(artifact => `
    <div class="download-entry"><strong>${escapeHTML(artifact.path)}</strong><span>${formatBytes(artifact.size)} · ${escapeHTML(artifact.verification_source)} · ${escapeHTML(artifact.sha256)}</span></div>
  `).join("") || "<p class=\"muted\">No indexed artifacts on this node.</p>";
}

function renderPlan(plan: {commit: string; destination: string; total_bytes: number; unsafe_warning: boolean; files: {path: string; size: number; required: boolean; reason: string}[]}): string {
  return `
    <div class="download-entry">
      <strong>${escapeHTML(plan.commit)}</strong>
      <span>${escapeHTML(plan.destination)} · ${formatBytes(plan.total_bytes)}</span>
      ${plan.unsafe_warning ? "<p class=\"error-text\">Hugging Face reports unsafe or pending security status. Starting requires confirmation.</p>" : ""}
      <ul>${plan.files.map(file => `<li>${escapeHTML(file.path)} · ${formatBytes(file.size)} · ${escapeHTML(file.reason)}${file.required ? " · required" : ""}</li>`).join("")}</ul>
    </div>
  `;
}

function renderJob(job: DownloadJob): string {
  const actions = job.state === "running" ? "pause cancel" : job.state === "paused" || job.state === "failed" ? "resume cancel" : "";
  return `
    <div class="download-entry">
      <strong>${escapeHTML(job.repository)} · ${escapeHTML(job.state)}</strong>
      <span>${formatBytes(job.completed_bytes)} / ${formatBytes(job.total_bytes)}</span>
      ${job.error ? `<p class="error-text">${escapeHTML(job.error)}</p>` : ""}
      <div class="button-strip">${actions.split(" ").filter(Boolean).map(action => `<button type="button" data-download-job="${escapeAttribute(job.id)}" data-download-action="${escapeAttribute(action)}">${escapeHTML(action)}</button>`).join("")}</div>
    </div>
  `;
}

function requestedFiles(): string[] {
  return elements.downloadFilesInput.value.split(/\r?\n/).map(value => value.trim()).filter(Boolean);
}

function downloadToken(): string | undefined {
  const token = elements.downloadTokenInput.value.trim();
  return token || undefined;
}
