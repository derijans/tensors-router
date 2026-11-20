import { toggledMember } from "../../lib/collections";
import { state } from "../../state/app-state";
import { renderDownloads } from "./downloads";
import { hfFilterCatalog } from "./hf-filter-catalog-data";

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
  state.downloads.filters = toggledMember(state.downloads.filters, filter);
  renderDownloads();
}

export function toggleDownloadFilterGroup(groupID: string): void {
  const key = `${state.downloads.filterTab}:${groupID}`;
  state.downloads.expandedFilterGroups = toggledMember(state.downloads.expandedFilterGroups, key);
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
