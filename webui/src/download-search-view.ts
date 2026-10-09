import { SafeHTML, emptyHTML, html, listOrFallback, setHTML } from "./safe-html";
import { badge } from "./markup-primitives";
import { elements } from "./elements";
import { selectedDownloadBytes } from "./download-plan-data";
import { hfFilterCatalog, hfFilterCatalogVersion } from "./hf-filter-catalog";
import { state } from "./state";
import { formatBytes } from "./utils";
import type { DownloadPlan } from "./types";

type DownloadCandidate = typeof state.downloads.candidates[number];
type DownloadSearchResult = typeof state.downloads.search[number];
type PlannedFile = DownloadPlan["files"][number];

const collapsedFilterGroupSize = 10;
const shownResultTagLimit = 6;
const activeClassAttribute = html` class="active"`;

export function renderSearchResults(): SafeHTML {
  const finder = state.downloads.finderMessage ? html`<p class="action-status">${state.downloads.finderMessage}</p>` : emptyHTML;
  const candidates = html`${state.downloads.candidates.map((candidate, index) => candidateEntry(candidate, index))}`;

  const rows = html`${state.downloads.search.map(searchResultEntry)}`;
  return html`${finder}${searchStatusNotice()}${rows}${candidates}`;
}

function candidateEntry(candidate: DownloadCandidate, index: number): SafeHTML {
  return html`
    <div class="download-entry candidate-${candidate.state}">
      <strong>${candidate.repository} / ${candidate.repository_path}</strong>
      <span>${candidate.state} · ${candidate.sha256 || "no verifiable LFS SHA-256"}</span>
      ${candidateActions(candidate, index)}
    </div>
  `;
}

function candidateActions(candidate: DownloadCandidate, index: number): SafeHTML {
  if (candidate.state === "exact") {
    return html`<button type="button" data-download-candidate-bind="${index}">Bind verified origin</button>`;
  }
  if (candidate.state === "mismatched" && state.downloads.modelHandoff && candidate.sha256) {
    return html`<button type="button" class="danger" data-download-candidate-replace="${index}">Replace expected model</button>`;
  }
  return emptyHTML;
}

function searchStatusNotice(): SafeHTML {
  switch (state.downloads.searchStatus) {
    case "searching":
      return html`<p class="action-status">Searching Hugging Face…</p>`;
    case "error":
      return html`<p class="error-text">Search failed: ${state.downloads.searchError || "unknown error"}</p>`;
    case "empty":
      return html`<p class="muted">No models match ${emptySearchSubject()}.</p>`;
    default:
      return emptyHTML;
  }
}

function emptySearchSubject(): string {
  const query = state.downloads.searchQuery;
  return query ? `"${query}"` : "the current filters";
}

function searchResultEntry(result: DownloadSearchResult): SafeHTML {
  const selected = state.downloads.selectedRepository === result.id ? " selected" : "";
  return html`
    <button class="download-entry${selected}" type="button" data-download-repository="${result.id}">
      <strong>${result.id}</strong>
      <span>${searchResultMeta(result).join(" · ")}</span>
      ${searchResultTags(result)}
    </button>`;
}

function searchResultMeta(result: DownloadSearchResult): string[] {
  const meta = [`${result.downloads.toLocaleString()} downloads`, `${result.likes.toLocaleString()} likes`];
  if (result.updated_at) {
    meta.push(`updated ${formatSearchDate(result.updated_at)}`);
  }
  if (result.gated && result.gated !== "false") {
    meta.push("gated");
  }
  return meta;
}

function searchResultTags(result: DownloadSearchResult): SafeHTML {
  const tags = (result.tags || []).filter(tag => !tag.includes(":") || /^(license|pipeline_tag|library):/.test(tag)).slice(0, shownResultTagLimit);
  if (tags.length === 0) {
    return emptyHTML;
  }
  return html`<span class="download-tags">${tags.map(tag => badge(tag, "neutral"))}</span>`;
}

function formatSearchDate(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toISOString().slice(0, 10);
}

export function renderDownloadFilters(): void {
  const activeTab = state.downloads.filterTab;
  const query = elements.downloadFilterSearch.value.trim().toLocaleLowerCase();
  const groups = [...(hfFilterCatalog[activeTab] || [])];
  if (activeTab === "main" && state.downloads.observedFilters.length > 0) {
    groups.push({id: "observed", label: "From current results", values: state.downloads.observedFilters});
  }
  setHTML(elements.downloadFilterTabs, html`${Object.keys(hfFilterCatalog).map(tab => filterTabButton(tab, tab === activeTab))}`);
  elements.downloadFilterOptions.dataset.catalogVersion = String(hfFilterCatalogVersion);
  const renderedGroups = groups.map(group => renderFilterGroup(activeTab, group.id, group.label, group.values, query)).filter(group => !group.isEmpty());
  setHTML(elements.downloadFilterOptions, listOrFallback(renderedGroups, html`<span class="muted">No filters match.</span>`));
  setHTML(elements.downloadFilterSummary, selectedFilterSummary());
}

function filterTabButton(tab: string, active: boolean): SafeHTML {
  return html`<button type="button" data-download-filter-tab="${tab}"${active ? activeClassAttribute : emptyHTML}>${tab}</button>`;
}

function selectedFilterSummary(): SafeHTML {
  if (state.downloads.filters.length === 0) {
    return html`<span class="muted">No metadata filters selected.</span>`;
  }
  return html`${state.downloads.filters.map(selectedFilterBadge)}<button type="button" class="badge tone-neutral" data-download-filter-clear-all>Clear all</button>`;
}

function selectedFilterBadge(filter: string): SafeHTML {
  return html`<button type="button" class="badge tone-accent" data-download-filter-clear="${filter}">${filter} ×</button>`;
}

function renderFilterGroup(activeTab: string, groupID: string, label: string, values: string[], query: string): SafeHTML {
  const matching = values.filter(value => !query || value.toLocaleLowerCase().includes(query));
  if (matching.length === 0) {
    return emptyHTML;
  }
  const expanded = query.length > 0 || state.downloads.expandedFilterGroups.includes(`${activeTab}:${groupID}`);
  const visible = expanded ? matching : matching.slice(0, collapsedFilterGroupSize);
  const toggle = filterGroupToggle(groupID, matching.length - visible.length, expanded && matching.length > collapsedFilterGroupSize);
  return html`<section class="filter-group"><h4>${label}</h4><div class="filter-group-options">${visible.map(filterChip)}${toggle}</div></section>`;
}

function filterGroupToggle(groupID: string, hiddenCount: number, collapsible: boolean): SafeHTML {
  if (hiddenCount > 0) {
    return html`<button type="button" class="filter-chip" data-download-filter-group="${groupID}">+${hiddenCount} more</button>`;
  }
  if (collapsible) {
    return html`<button type="button" class="filter-chip" data-download-filter-group="${groupID}">Show less</button>`;
  }
  return emptyHTML;
}

function filterChip(filter: string): SafeHTML {
  const active = state.downloads.filters.includes(filter) ? " active" : "";
  return html`<button type="button" class="filter-chip${active}" data-download-filter="${filter}">${filterLabel(filter)}</button>`;
}

function filterLabel(value: string): string {
  return value.replace(/^(?:app|provider|dataset|library|language|license):/, "");
}

export function renderPlan(plan: DownloadPlan): SafeHTML {
  const selected = new Set(state.downloads.selectedPlanFiles);
  const selectedBytes = selectedDownloadBytes(plan, state.downloads.selectedPlanFiles);
  return html`
    <div class="download-entry">
      <strong>${plan.commit}</strong>
      <span>${plan.destination} · ${formatBytes(selectedBytes)} selected of ${formatBytes(plan.total_bytes)}</span>
      ${plan.unsafe_warning ? html`<p class="error-text">Hugging Face reports unsafe or pending security status. Starting requires confirmation.</p>` : ""}
      ${plan.gated ? html`<p class="action-status">Gated repository: approve access on Hugging Face and use an authorized token.</p>` : ""}
      ${skippedFilesNotice(plan)}
      <div class="button-strip">
        <button type="button" data-download-plan-select="all">Select all</button>
        <button type="button" data-download-plan-select="none">Select none</button>
        <button type="button" data-download-plan-select="required">Required only</button>
      </div>
      <ul class="plan-files">${plan.files.map(file => plannedFileItem(file, selected.has(file.path)))}</ul>
    </div>
  `;
}

function skippedFilesNotice(plan: DownloadPlan): SafeHTML {
  const skipped = plan.skipped || [];
  if (skipped.length === 0) {
    return emptyHTML;
  }
  return html`<p class="muted">Skipped: ${skipped.map(skippedFileLabel).join(", ")}</p>`;
}

function skippedFileLabel(file: NonNullable<DownloadPlan["skipped"]>[number]): string {
  return `${file.path} (${file.reason})`;
}

function plannedFileItem(file: PlannedFile, checked: boolean): SafeHTML {
  return html`<li><label class="toggle-row"><input type="checkbox" data-download-plan-file="${file.path}"${checked ? " checked" : ""}><code>${file.path}</code><span class="muted">${formatBytes(file.size)} · ${file.reason}</span>${file.required ? badge("required", "accent") : ""}</label></li>`;
}
