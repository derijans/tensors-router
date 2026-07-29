import {
  createDownloadJob,
  downloadJobAction,
  getDownloadCapabilities,
  getDownloadLibrary,
  bindModelAssetCandidate,
  findModelAssetCandidates,
  lookupModelAsset,
  loadModelConfig,
  planDownload,
  rescanDownloads,
  searchDownloadPage,
  substituteModelAsset
} from "./api";
import { elements } from "./elements";
import { normalizeModelHash, normalizeParameterRange, parseOfficialHFURL, splitSearchFilters } from "./download-finder-data";
import { selectedDownloadBytes, selectedDownloadFiles, toggleDownloadPath } from "./download-plan-data";
import { hfFilterCatalog, hfFilterCatalogVersion } from "./hf-filter-catalog";
import { state } from "./state";
import { escapeAttribute, escapeHTML, formatBytes } from "./utils";
import type { DownloadJob, DownloadPlan } from "./types";

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

let searchController: AbortController | null = null;
let searchDebounce: number | undefined;

export async function searchDownloadRepositories(append = false): Promise<void> {
  if (!state.downloads.nodeID) {
    throw new Error("Select a download node first");
  }
  const token = downloadToken();
  const mode = elements.downloadSearchMode.value;
  const query = elements.downloadSearchInput.value.trim();
  if (mode !== "text") {
    searchController?.abort();
    searchController = new AbortController();
    await runSpecialFinderMode(mode, query, token, searchController.signal);
    renderDownloads();
    return;
  }
  const parameters = parameterRange();
  const rawTags = elements.downloadRawTagInput.value.split(",").map(value => value.trim()).filter(Boolean);
  const searchFilters = splitSearchFilters([...state.downloads.filters, ...rawTags]);
  searchController?.abort();
  searchController = new AbortController();
  const page = await searchDownloadPage({
    node_id: state.downloads.nodeID,
    query,
    ...(elements.downloadAuthorInput.value.trim() ? {author: elements.downloadAuthorInput.value.trim()} : {}),
    ...(elements.downloadPipelineInput.value.trim() ? {pipeline_tag: elements.downloadPipelineInput.value.trim()} : {}),
    filters: searchFilters.filters,
    apps: searchFilters.apps,
    inference_providers: searchFilters.providers,
    trained_datasets: searchFilters.datasets,
    ...(searchFilters.inference ? {inference: "true"} : {}),
    sort: elements.downloadSortSelect.value,
    direction: elements.downloadDirectionSelect.value,
    ...(append && state.downloads.nextCursor ? {cursor: state.downloads.nextCursor} : {}),
    limit: 20,
    ...(elements.downloadGatedSelect.value ? {gated: elements.downloadGatedSelect.value} : {}),
    ...(parameters ? {num_parameters: parameters} : {}),
    ...(token ? {token} : {})
  }, searchController.signal);
  state.downloads.search = append ? [...state.downloads.search, ...page.results] : page.results;
  state.downloads.observedFilters = [...new Set([
    ...state.downloads.observedFilters,
    ...page.results.flatMap(result => result.tags || []).filter(validObservedFilter)
  ])].slice(-160);
  state.downloads.nextCursor = page.next_cursor || "";
  renderDownloads();
}

export function updateDownloadSearchMode(): void {
  const placeholders: Record<string, string> = {
    text: "Repository or author",
    url: "https://huggingface.co/owner/repository",
    hash: "Lowercase SHA-256",
    filename: "Exact model filename"
  };
  elements.downloadSearchInput.placeholder = placeholders[elements.downloadSearchMode.value] || "Repository or author";
  state.downloads.search = [];
  state.downloads.candidates = [];
  state.downloads.nextCursor = "";
  state.downloads.finderMessage = "";
  renderDownloads();
}

export function prefillDownloadContext(context: {nodeID: string; publicID: string; configID: string; configFilename: string; field: string; position?: number; filename: string; hash: string}): void {
  if (state.downloads.capabilities?.nodes.some(node => node.node_id === context.nodeID)) {
    state.downloads.nodeID = context.nodeID;
  }
  state.downloads.modelHandoff = context;
  elements.downloadSearchMode.value = "filename";
  elements.downloadSearchInput.value = context.filename;
  elements.downloadExpectedHashInput.value = context.hash;
  updateDownloadSearchMode();
  state.downloads.finderMessage = "Prefilled from unresolved Models load. Search to verify repository candidates.";
  renderDownloads();
}

export async function replaceDownloadCandidate(index: number): Promise<void> {
  const candidate = state.downloads.candidates[index];
  const context = state.downloads.modelHandoff;
  if (!candidate || candidate.state !== "mismatched" || !candidate.sha256 || !context) {
    throw new Error("A verified mismatching Models candidate is required");
  }
  const token = downloadToken();
  await substituteModelAsset({
    node_id: context.nodeID,
    id: context.configID,
    filename: context.configFilename,
    field: context.field,
    ...(context.position === undefined ? {} : {position: context.position}),
    expected_sha256: context.hash,
    sha256: normalizeModelHash(candidate.sha256),
    repository: candidate.repository,
    repository_path: candidate.repository_path,
    commit: candidate.commit,
    ...(token ? {token} : {}),
    confirm: true
  });
  state.downloads.modelHandoff = null;
  state.downloads.finderMessage = `Config intentionally updated to ${candidate.repository_path}; loading model`;
  renderDownloads();
  await loadModelConfig({model: context.publicID});
  state.downloads.finderMessage = `Config intentionally updated to ${candidate.repository_path} and loaded`;
  renderDownloads();
}

export async function bindDownloadCandidate(index: number): Promise<void> {
  const candidate = state.downloads.candidates[index];
  const hash = normalizedExpectedHash();
  if (!candidate || candidate.state !== "exact" || !state.downloads.nodeID) {
    throw new Error("Only an exact verified candidate can be bound");
  }
  const token = downloadToken();
  await bindModelAssetCandidate({
    node_id: state.downloads.nodeID,
    sha256: hash,
    repository: candidate.repository,
    repository_path: candidate.repository_path,
    commit: candidate.commit,
    ...(token ? {token} : {})
  });
  state.downloads.finderMessage = `Verified origin bound for ${candidate.repository_path}`;
  renderDownloads();
}

export function debounceDownloadSearch(): void {
  if (searchDebounce !== undefined) {
    window.clearTimeout(searchDebounce);
  }
  searchDebounce = window.setTimeout(() => {
    void searchDownloadRepositories(false).catch(error => {
      if (error instanceof DOMException && error.name === "AbortError") {
        return;
      }
      state.downloads.error = error instanceof Error ? error.message : String(error);
      renderDownloads();
    });
  }, 300);
}

export function selectDownloadFilterTab(tab: string): void {
  if (hfFilterCatalog[tab]) {
    state.downloads.filterTab = tab;
    renderDownloads();
  }
}

export function toggleDownloadFilter(filter: string): void {
  if (!allAvailableFilters().has(filter)) {
    return;
  }
  state.downloads.filters = state.downloads.filters.includes(filter)
    ? state.downloads.filters.filter(value => value !== filter)
    : [...state.downloads.filters, filter];
  renderDownloads();
}

export function toggleDownloadFilterGroup(groupID: string): void {
  const key = `${state.downloads.filterTab}:${groupID}`;
  state.downloads.expandedFilterGroups = state.downloads.expandedFilterGroups.includes(key)
    ? state.downloads.expandedFilterGroups.filter(value => value !== key)
    : [...state.downloads.expandedFilterGroups, key];
  renderDownloads();
}

export function updateDownloadFilterSearch(): void {
  renderDownloads();
}

export function clearDownloadFilter(filter: string): void {
  state.downloads.filters = state.downloads.filters.filter(value => value !== filter);
  renderDownloads();
}

async function runSpecialFinderMode(mode: string, query: string, token: string | undefined, signal: AbortSignal): Promise<void> {
  state.downloads.search = [];
  state.downloads.candidates = [];
  state.downloads.nextCursor = "";
  state.downloads.finderMessage = "";
  if (mode === "url") {
    const parsed = parseOfficialHFURL(query);
    elements.downloadRepositoryInput.value = parsed.repository;
    elements.downloadRevisionInput.value = parsed.revision;
    if (parsed.file) {
      elements.downloadFilesInput.value = parsed.file;
    }
    state.downloads.search = [{id: parsed.repository, downloads: 0, likes: 0}];
    state.downloads.finderMessage = parsed.file ? "Official Hugging Face file URL parsed" : "Official Hugging Face repository URL parsed";
    return;
  }
  if (mode === "hash") {
    const hash = normalizeModelHash(query);
    const result = await lookupModelAsset(hash, signal);
    state.downloads.finderMessage = `${result.available ? `Available on ${(result.nodes || []).length} router node(s)` : "Not locally available"}${result.origin ? ` · learned origin ${result.origin}` : ""}`;
    if (result.origin) {
      const parsed = parseOfficialHFURL(result.origin);
      elements.downloadRepositoryInput.value = parsed.repository;
      elements.downloadRevisionInput.value = parsed.revision;
      elements.downloadFilesInput.value = parsed.file;
      state.downloads.search = [{id: parsed.repository, downloads: 0, likes: 0}];
    }
    return;
  }
  if (mode === "filename") {
    if (!state.downloads.nodeID) {
      throw new Error("Select a download node first");
    }
    if (!/^[^/\\\0]{1,255}$/.test(query) || query === "." || query === "..") {
      throw new Error("Enter a safe exact filename");
    }
    const hash = normalizedExpectedHash();
    state.downloads.candidates = await findModelAssetCandidates({node_id: state.downloads.nodeID, sha256: hash, filename: query, ...(token ? {token} : {})}, signal);
    const exact = state.downloads.candidates.filter(candidate => candidate.state === "exact").length;
    state.downloads.finderMessage = `${state.downloads.candidates.length} candidate file(s), ${exact} exact SHA-256 match${exact === 1 ? "" : "es"}`;
    return;
  }
  throw new Error("Unsupported search mode");
}

function normalizedExpectedHash(): string {
  return normalizeModelHash(elements.downloadExpectedHashInput.value.trim());
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
    revision: plan.revision,
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
  elements.downloadSearchResults.innerHTML = "";
}

export function togglePlannedDownloadFile(path: string): void {
  const plan = state.downloads.plan;
  if (!plan || !plan.files.some(file => file.path === path)) {
    return;
  }
  state.downloads.selectedPlanFiles = toggleDownloadPath(state.downloads.selectedPlanFiles, path);
  renderDownloads();
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
  elements.downloadStartButton.disabled = state.downloads.plan === null || state.downloads.selectedPlanFiles.length === 0;
  renderDownloadFilters();
  elements.downloadSearchResults.innerHTML = `${state.downloads.finderMessage ? `<p class="action-status">${escapeHTML(state.downloads.finderMessage)}</p>` : ""}${state.downloads.search.map(result => `
    <button class="download-entry" type="button" data-download-repository="${escapeAttribute(result.id)}">
      <strong>${escapeHTML(result.id)}</strong><span>${result.downloads} downloads · ${result.likes} likes${result.gated ? " · gated" : ""}</span>
    </button>
  `).join("")}${state.downloads.candidates.map((candidate, index) => `
    <div class="download-entry candidate-${escapeAttribute(candidate.state)}">
      <strong>${escapeHTML(candidate.repository)} / ${escapeHTML(candidate.repository_path)}</strong>
      <span>${escapeHTML(candidate.state)} · ${escapeHTML(candidate.sha256 || "no verifiable LFS SHA-256")}</span>
      ${candidate.state === "exact" ? `<button type="button" data-download-candidate-bind="${index}">Bind verified origin</button>` : ""}
      ${candidate.state === "mismatched" && state.downloads.modelHandoff && candidate.sha256 ? `<button type="button" class="danger" data-download-candidate-replace="${index}">Replace expected model</button>` : ""}
    </div>
  `).join("")}`;
  elements.downloadNextPageButton.hidden = !state.downloads.nextCursor;
  elements.downloadPlanOutput.innerHTML = state.downloads.plan ? renderPlan(state.downloads.plan) : "";
  elements.downloadJobs.innerHTML = (state.downloads.library?.jobs || []).map(renderJob).join("") || "<p class=\"muted\">No download jobs on this node.</p>";
  elements.downloadLibrary.innerHTML = (state.downloads.library?.artifacts || []).map(artifact => `
    <div class="download-entry"><strong>${escapeHTML(artifact.path)}</strong><span>${formatBytes(artifact.size)} · ${escapeHTML(artifact.verification_source)} · ${escapeHTML(artifact.sha256)}</span></div>
  `).join("") || "<p class=\"muted\">No indexed artifacts on this node.</p>";
}

function renderDownloadFilters(): void {
  const activeTab = state.downloads.filterTab;
  const query = elements.downloadFilterSearch.value.trim().toLocaleLowerCase();
  const groups = [...(hfFilterCatalog[activeTab] || [])];
  if (activeTab === "main" && state.downloads.observedFilters.length > 0) {
    groups.push({id: "observed", label: "From current results", values: state.downloads.observedFilters});
  }
  elements.downloadFilterTabs.innerHTML = Object.keys(hfFilterCatalog).map(tab => `<button type="button" data-download-filter-tab="${escapeAttribute(tab)}"${tab === activeTab ? " class=\"active\"" : ""}>${escapeHTML(tab)}</button>`).join("");
  elements.downloadFilterOptions.dataset.catalogVersion = String(hfFilterCatalogVersion);
  elements.downloadFilterOptions.innerHTML = groups.map(group => renderFilterGroup(activeTab, group.id, group.label, group.values, query)).join("") || "<span class=\"muted\">No filters match.</span>";
  elements.downloadFilterSummary.innerHTML = state.downloads.filters.length === 0 ? "<span class=\"muted\">No metadata filters selected.</span>" : state.downloads.filters.map(filter => `<button type="button" class="chip" data-download-filter-clear="${escapeAttribute(filter)}">${escapeHTML(filter)} ×</button>`).join("");
}

function renderFilterGroup(activeTab: string, groupID: string, label: string, values: string[], query: string): string {
  const matching = values.filter(value => !query || value.toLocaleLowerCase().includes(query));
  if (matching.length === 0) {
    return "";
  }
  const expanded = query.length > 0 || state.downloads.expandedFilterGroups.includes(`${activeTab}:${groupID}`);
  const visible = expanded ? matching : matching.slice(0, 10);
  const remaining = matching.length - visible.length;
  const toggle = remaining > 0
    ? `<button type="button" class="filter-chip" data-download-filter-group="${escapeAttribute(groupID)}">+${remaining} more</button>`
    : expanded && matching.length > 10
      ? `<button type="button" class="filter-chip" data-download-filter-group="${escapeAttribute(groupID)}">Show less</button>`
      : "";
  return `<section class="filter-group"><h4>${escapeHTML(label)}</h4><div class="filter-group-options">${visible.map(filter => `<button type="button" class="filter-chip${state.downloads.filters.includes(filter) ? " active" : ""}" data-download-filter="${escapeAttribute(filter)}">${escapeHTML(filterLabel(filter))}</button>`).join("")}${toggle}</div></section>`;
}

function allAvailableFilters(): Set<string> {
  return new Set([
    ...Object.values(hfFilterCatalog).flatMap(groups => groups.flatMap(group => group.values)),
    ...state.downloads.observedFilters
  ]);
}

function validObservedFilter(value: string): boolean {
  return value.length > 0 && value.length <= 128 && /^[\w.+:/-]+$/u.test(value);
}

function filterLabel(value: string): string {
  return value.replace(/^(?:app|provider|dataset|library|language|license):/, "");
}

function renderPlan(plan: DownloadPlan): string {
  const selected = new Set(state.downloads.selectedPlanFiles);
  const selectedBytes = selectedDownloadBytes(plan, state.downloads.selectedPlanFiles);
  return `
    <div class="download-entry">
      <strong>${escapeHTML(plan.commit)}</strong>
      <span>${escapeHTML(plan.destination)} · ${formatBytes(selectedBytes)} selected of ${formatBytes(plan.total_bytes)}</span>
      ${plan.unsafe_warning ? "<p class=\"error-text\">Hugging Face reports unsafe or pending security status. Starting requires confirmation.</p>" : ""}
      <ul>${plan.files.map(file => `<li><label><input type="checkbox" data-download-plan-file="${escapeAttribute(file.path)}"${selected.has(file.path) ? " checked" : ""}> ${escapeHTML(file.path)} · ${formatBytes(file.size)} · ${escapeHTML(file.reason)}${file.required ? " · required" : ""}</label></li>`).join("")}</ul>
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

function parameterRange(): string | undefined {
  return normalizeParameterRange(elements.downloadParameterMin.value.trim(), elements.downloadParameterMax.value.trim());
}
