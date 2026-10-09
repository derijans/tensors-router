import { SafeHTML, emptyHTML, html, setHTML } from "./safe-html";
import { createModelAssetResolutionJob, getModelAssetResolutionJob, loadModelConfig } from "./api";
import type { ModelAssetResolutionJob } from "./api";
import { filterInventoryModels, inventoryModels } from "./model-inventory-data";
import { modelAssetHandoff } from "./model-asset-handoff";
import { elements } from "./elements";
import { state } from "./state";

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
    await handoffUnresolvedModel(id, refreshInventory);
    throw error;
  }
}

async function handoffUnresolvedModel(id: string, refreshInventory: () => Promise<void>): Promise<void> {
  await refreshInventory().catch(() => undefined);
  const model = state.inventory?.models.find(value => value.public_id === id || value.local_id === id);
  const detail = modelAssetHandoff(model);
  if (!detail) {
    return;
  }
  window.dispatchEvent(new CustomEvent("model-asset-handoff", {detail}));
}

export async function resolveFilteredModels(refreshInventory: () => Promise<void>): Promise<void> {
  const models = filterInventoryModels(inventoryModels(state.inventory?.models ?? [], state.inventory?.nodes ?? []), {
    query: state.models.modelSearch,
    nodeIDs: state.models.configNodeIDs,
    enabled: state.models.enabledFilter,
    backend: state.models.backendFilter,
    capability: state.models.capabilityFilter
  });
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
  const failedCount = results.filter(resolutionFailed).length;
  elements.modelsActionStatus.classList.toggle("error-text", results.length === total && failedCount > 0);
  setHTML(elements.modelsActionStatus, html`<p>${resolutionSummary(results.length, failedCount, total)}</p><div class="resolution-results">${results.map(resolutionResultLine)}</div>`);
}

function resolutionFailed(result: ResolutionOutcome): boolean {
  return Boolean(result.error) || result.job?.state === "failed";
}

function resolutionSummary(finished: number, failedCount: number, total: number): string {
  if (finished < total) {
    return `Resolved ${finished} of ${total} visible configs...`;
  }
  const completed = finished - failedCount;
  return failedCount > 0 ? `${completed} resolved, ${failedCount} failed` : `${completed} visible configs resolved`;
}

function resolutionResultLine(result: ResolutionOutcome): SafeHTML {
  return html`<div class="resolution-result"><span>${result.request.id} · ${resolutionFieldSummary(result)}</span>${resolutionRetryButton(result)}</div>`;
}

function resolutionRetryButton(result: ResolutionOutcome): SafeHTML {
  if (!resolutionFailed(result)) {
    return emptyHTML;
  }
  return html`<button type="button" data-model-resolution-retry="${resolutionRequestKey(result.request)}">Retry</button>`;
}

function resolutionFieldSummary(result: ResolutionOutcome): string {
  return result.job?.results?.map(resolvedFieldLabel).join(", ") || result.error || result.job?.state || "completed";
}

function resolvedFieldLabel(field: NonNullable<ModelAssetResolutionJob["results"]>[number]): string {
  const outcome = field.resolved ? field.source || "verified" : field.failure || "unavailable";
  return `${field.field}: ${outcome}`;
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
