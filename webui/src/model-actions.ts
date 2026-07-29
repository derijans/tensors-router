import { createModelAssetResolutionJob, getModelAssetResolutionJob, loadModelConfig } from "./api";
import type { ModelAssetResolutionJob } from "./api";
import { visibleModelsForResolution } from "./data";
import { elements } from "./elements";
import { state } from "./state";
import { escapeAttribute, escapeHTML } from "./utils";

export async function loadSelectedConfig(modelID: string, refreshInventory: () => Promise<void>): Promise<void> {
  const id = modelID.trim();
  if (!id) {
    return;
  }
  setModelActionStatus(`Loading ${id}...`, false);
  try {
    await loadModelConfig({model: id});
    setModelActionStatus(`Loaded ${id}`, false);
    await refreshInventory();
  } catch (error) {
    setModelActionStatus(error instanceof Error ? error.message : String(error), true);
    handoffUnresolvedModel(id);
    throw error;
  }
}

function handoffUnresolvedModel(id: string): void {
  const model = state.inventory?.models.find(value => value.public_id === id || value.local_id === id);
  const options = model?.options || {};
  for (const [key, value] of Object.entries(options)) {
    if (!key.endsWith("_hash")) {
      continue;
    }
    const field = key.slice(0, -5);
    const filenameValue = options[`${field}_filename`];
    const hashes = Array.isArray(value) ? value : [value];
    const filenames = Array.isArray(filenameValue) ? filenameValue : [filenameValue];
    const position = hashes.findIndex(hash => typeof hash === "string" && /^[0-9a-f]{64}$/.test(hash));
    const hash = hashes[position];
    const filename = filenames[position];
    if (position >= 0 && typeof hash === "string" && typeof filename === "string" && filename.length > 0 && model) {
      window.dispatchEvent(new CustomEvent("model-asset-handoff", {detail: {
        nodeID: model.node_id || "",
        publicID: model.public_id || model.local_id,
        configID: model.local_id,
        configFilename: model.filename,
        field,
        ...(Array.isArray(value) ? {position} : {}),
        filename,
        hash
      }}));
      return;
    }
  }
}

export async function resolveFilteredModels(refreshInventory: () => Promise<void>): Promise<void> {
  const models = visibleModelsForResolution(elements.filterInput.value.trim().toLowerCase(), state.models.configNodeIDs);
  if (models.length === 0) {
    setModelActionStatus("No visible configs", false);
    return;
  }
  const requests = models.map(model => ({node_id: model.node_id || "", ...(model.node_url ? {node_url: model.node_url} : {}), id: model.local_id, filename: model.filename}));
  requests.forEach(request => resolutionRequests.set(resolutionRequestKey(request), request));
  const results: ResolutionOutcome[] = [];
  renderResolutionProgress(results, requests.length);
  const nodeGroups = groupRequestsByNode(requests);
  await Promise.all([...nodeGroups.values()].map(group => runResolutionQueue(group, results, requests.length)));
  renderResolutionProgress(results, requests.length);
  await refreshInventory();
}

export async function retryModelAssetResolution(key: string, refreshInventory: () => Promise<void>): Promise<void> {
  const request = resolutionRequests.get(key);
  if (!request) {
    throw new Error("Resolution request is no longer available");
  }
  const result = await resolveRequest(request);
  renderResolutionProgress([result], 1);
  await refreshInventory();
}

interface ResolutionRequest {
  node_id: string;
  node_url?: string;
  id: string;
  filename: string;
}

interface ResolutionOutcome {
  request: ResolutionRequest;
  job?: ModelAssetResolutionJob;
  error?: string;
}

const resolutionRequests = new Map<string, ResolutionRequest>();

async function runResolutionQueue(requests: ResolutionRequest[], results: ResolutionOutcome[], total: number): Promise<void> {
  let next = 0;
  const worker = async (): Promise<void> => {
    while (next < requests.length) {
      const request = requests[next++]!;
      results.push(await resolveRequest(request));
      renderResolutionProgress(results, total);
    }
  };
  await Promise.all(Array.from({length: Math.min(2, requests.length)}, worker));
}

async function resolveRequest(request: ResolutionRequest): Promise<ResolutionOutcome> {
  try {
    let job = await createModelAssetResolutionJob(request);
    while (job.state === "queued" || job.state === "resolving") {
      await waitForPoll();
      job = await getModelAssetResolutionJob(request.node_id, job.id);
    }
    return {request, job};
  } catch (error) {
    return {request, error: error instanceof Error ? error.message : String(error)};
  }
}

function renderResolutionProgress(results: ResolutionOutcome[], total: number): void {
  const failed = results.filter(result => result.error || result.job?.state === "failed");
  const completed = results.length - failed.length;
  const summary = results.length < total
    ? `Resolved ${results.length} of ${total} visible configs...`
    : failed.length > 0
      ? `${completed} resolved, ${failed.length} failed`
      : `${completed} visible configs resolved`;
  elements.modelsActionStatus.classList.toggle("error-text", results.length === total && failed.length > 0);
  elements.modelsActionStatus.innerHTML = `<p>${escapeHTML(summary)}</p><div class="resolution-results">${results.map(result => {
    const failedResult = result.error || result.job?.state === "failed";
    const fieldSummary = result.job?.results?.map(field => `${field.field}: ${field.resolved ? field.source || "verified" : field.failure || "unavailable"}`).join(", ") || result.error || result.job?.state || "completed";
    return `<div class="resolution-result"><span>${escapeHTML(result.request.id)} · ${escapeHTML(fieldSummary)}</span>${failedResult ? `<button type="button" data-model-resolution-retry="${escapeAttribute(resolutionRequestKey(result.request))}">Retry</button>` : ""}</div>`;
  }).join("")}</div>`;
}

function resolutionRequestKey(request: ResolutionRequest): string {
  return `${encodeURIComponent(request.node_id)}|${encodeURIComponent(request.id)}`;
}

function groupRequestsByNode(requests: ResolutionRequest[]): Map<string, ResolutionRequest[]> {
  const groups = new Map<string, ResolutionRequest[]>();
  for (const request of requests) {
    const group = groups.get(request.node_id) || [];
    group.push(request);
    groups.set(request.node_id, group);
  }
  return groups;
}

function waitForPoll(): Promise<void> {
  return new Promise(resolve => window.setTimeout(resolve, 250));
}

export function setModelActionStatus(message: string, error: boolean): void {
  elements.modelsActionStatus.textContent = message;
  elements.modelsActionStatus.classList.toggle("error-text", error);
}
