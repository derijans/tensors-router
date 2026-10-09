import {
  bindModelAssetCandidate,
  findModelAssetCandidates,
  loadModelConfig,
  lookupModelAsset,
  searchDownloadPage,
  substituteModelAsset
} from "./api";
import { elements } from "./elements";
import { normalizeModelHash, parseOfficialHFURL, splitSearchFilters } from "./download-finder-data";
import { downloadToken, normalizedExpectedHash, parameterRange } from "./download-inputs";
import { renderDownloads } from "./downloads";
import { hfFilterCatalog } from "./hf-filter-catalog";
import { state } from "./state";

let searchController: AbortController | null = null;
let searchDebounce: number | undefined;

type DownloadSearchRequest = Parameters<typeof searchDownloadPage>[0];
type DownloadSearchPage = Awaited<ReturnType<typeof searchDownloadPage>>;
type ModelAssetLookup = Awaited<ReturnType<typeof lookupModelAsset>>;

const observedFilterLimit = 160;
const searchPageSize = 20;

export async function searchDownloadRepositories(append = false): Promise<void> {
  if (!state.downloads.nodeID) {
    throw new Error("Select a download node first");
  }
  const token = downloadToken();
  const mode = elements.downloadSearchMode.value;
  const query = elements.downloadSearchInput.value.trim();
  if (mode !== "text") {
    const signal = restartSearch();
    state.downloads.searchStatus = "idle";
    state.downloads.searchError = "";
    await runSpecialFinderMode(mode, query, token, signal);
    renderDownloads();
    return;
  }
  const request = textSearchRequest(query, append, token);
  const signal = restartSearch();
  beginTextSearch(query, append);
  try {
    acceptSearchPage(await searchDownloadPage(request, signal), append);
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") {
      state.downloads.searchStatus = state.downloads.search.length === 0 ? "idle" : "ok";
      return;
    }
    state.downloads.searchStatus = "error";
    state.downloads.searchError = error instanceof Error ? error.message : String(error);
    throw error;
  } finally {
    renderDownloads();
  }
}

function restartSearch(): AbortSignal {
  searchController?.abort();
  searchController = new AbortController();
  return searchController.signal;
}

function beginTextSearch(query: string, append: boolean): void {
  state.downloads.searchStatus = "searching";
  state.downloads.searchError = "";
  state.downloads.searchQuery = query;
  if (!append) {
    state.downloads.search = [];
  }
  renderDownloads();
}

function textSearchRequest(query: string, append: boolean, token: string | undefined): DownloadSearchRequest {
  const parameters = parameterRange();
  const rawTags = elements.downloadRawTagInput.value.split(",").map(value => value.trim()).filter(Boolean);
  const searchFilters = splitSearchFilters([...state.downloads.filters, ...rawTags]);
  const author = elements.downloadAuthorInput.value.trim();
  const pipelineTag = elements.downloadPipelineInput.value.trim();
  const gated = elements.downloadGatedSelect.value;
  const cursor = append ? state.downloads.nextCursor : "";
  return {
    node_id: state.downloads.nodeID,
    query,
    ...(author ? {author} : {}),
    ...(pipelineTag ? {pipeline_tag: pipelineTag} : {}),
    filters: searchFilters.filters,
    apps: searchFilters.apps,
    inference_providers: searchFilters.providers,
    trained_datasets: searchFilters.datasets,
    ...(searchFilters.inference ? {inference: "true"} : {}),
    sort: elements.downloadSortSelect.value,
    direction: elements.downloadDirectionSelect.value,
    ...(cursor ? {cursor} : {}),
    limit: searchPageSize,
    ...(gated ? {gated} : {}),
    ...(parameters ? {num_parameters: parameters} : {}),
    ...(token ? {token} : {})
  };
}

function acceptSearchPage(page: DownloadSearchPage, append: boolean): void {
  state.downloads.search = append ? [...state.downloads.search, ...page.results] : page.results;
  state.downloads.observedFilters = [...new Set([
    ...state.downloads.observedFilters,
    ...page.results.flatMap(result => result.tags || []).filter(validObservedFilter)
  ])].slice(-observedFilterLimit);
  state.downloads.nextCursor = page.next_cursor || "";
  state.downloads.searchStatus = state.downloads.search.length === 0 ? "empty" : "ok";
}

async function runSpecialFinderMode(mode: string, query: string, token: string | undefined, signal: AbortSignal): Promise<void> {
  state.downloads.search = [];
  state.downloads.candidates = [];
  state.downloads.nextCursor = "";
  state.downloads.finderMessage = "";
  switch (mode) {
    case "url":
      applyOfficialHFURL(query);
      return;
    case "hash":
      await lookupHashOrigin(query, signal);
      return;
    case "filename":
      await findFilenameCandidates(query, token, signal);
      return;
    default:
      throw new Error("Unsupported search mode");
  }
}

function applyOfficialHFURL(query: string): void {
  const parsed = parseOfficialHFURL(query);
  elements.downloadRepositoryInput.value = parsed.repository;
  elements.downloadRevisionInput.value = parsed.revision;
  if (parsed.file) {
    elements.downloadFilesInput.value = parsed.file;
  }
  state.downloads.search = [{id: parsed.repository, downloads: 0, likes: 0}];
  state.downloads.finderMessage = parsed.file ? "Official Hugging Face file URL parsed" : "Official Hugging Face repository URL parsed";
}

async function lookupHashOrigin(query: string, signal: AbortSignal): Promise<void> {
  const result = await lookupModelAsset(normalizeModelHash(query), signal);
  state.downloads.finderMessage = hashLookupMessage(result);
  if (!result.origin) {
    return;
  }
  const parsed = parseOfficialHFURL(result.origin);
  elements.downloadRepositoryInput.value = parsed.repository;
  elements.downloadRevisionInput.value = parsed.revision;
  elements.downloadFilesInput.value = parsed.file;
  state.downloads.search = [{id: parsed.repository, downloads: 0, likes: 0}];
}

function hashLookupMessage(result: ModelAssetLookup): string {
  const availability = result.available ? `Available on ${(result.nodes || []).length} router node(s)` : "Not locally available";
  const origin = result.origin ? ` · learned origin ${result.origin}` : "";
  return `${availability}${origin}`;
}

async function findFilenameCandidates(query: string, token: string | undefined, signal: AbortSignal): Promise<void> {
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
  if (candidate?.state !== "mismatched" || !candidate.sha256 || !context) {
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
  if (candidate?.state !== "exact" || !state.downloads.nodeID) {
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
    void searchDownloadRepositories(false).catch(() => undefined);
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

export function clearAllDownloadFilters(): void {
  state.downloads.filters = [];
  renderDownloads();
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
