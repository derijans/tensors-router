import { findModelAssetCandidates, lookupModelAsset } from "../../api/model-assets-api";
import { searchDownloadPage } from "../../api/downloads-api";
import { normalizeModelHash, parseOfficialHFURL, splitSearchFilters, validObservedFilter } from "./download-finder-data";
import { downloadToken, normalizedExpectedHash, parameterRange } from "./download-inputs";
import { renderDownloads } from "./downloads";
import { state } from "../../state/app-state";
import { errorMessage } from "../../lib/errors";
import { downloadsElements } from "./downloads-elements";

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
  const mode = downloadsElements.downloadSearchMode.value;
  const query = downloadsElements.downloadSearchInput.value.trim();
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
    state.downloads.searchError = errorMessage(error);
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
  const rawTags = downloadsElements.downloadRawTagInput.value.split(",").map(value => value.trim()).filter(Boolean);
  const searchFilters = splitSearchFilters([...state.downloads.filters, ...rawTags]);
  const author = downloadsElements.downloadAuthorInput.value.trim();
  const pipelineTag = downloadsElements.downloadPipelineInput.value.trim();
  const gated = downloadsElements.downloadGatedSelect.value;
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
    sort: downloadsElements.downloadSortSelect.value,
    direction: downloadsElements.downloadDirectionSelect.value,
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
  downloadsElements.downloadRepositoryInput.value = parsed.repository;
  downloadsElements.downloadRevisionInput.value = parsed.revision;
  if (parsed.file) {
    downloadsElements.downloadFilesInput.value = parsed.file;
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
  downloadsElements.downloadRepositoryInput.value = parsed.repository;
  downloadsElements.downloadRevisionInput.value = parsed.revision;
  downloadsElements.downloadFilesInput.value = parsed.file;
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
  downloadsElements.downloadSearchInput.placeholder = placeholders[downloadsElements.downloadSearchMode.value] || "Repository or author";
  state.downloads.search = [];
  state.downloads.candidates = [];
  state.downloads.nextCursor = "";
  state.downloads.finderMessage = "";
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
