import { setHTML } from "../../lib/safe-html";
import { state } from "../../state/app-state";
import { type FilterGroupView, type FilterPanelView, filterGroupsMarkup, filterTabsMarkup, selectedFilterSummary } from "./download-filters-view";
import { downloadsElements } from "./downloads-elements";
import { hfFilterCatalog, hfFilterCatalogVersion } from "./hf-filter-catalog-data";

export function renderDownloadFilters(): void {
  const view = filterPanelView();
  setHTML(downloadsElements.downloadFilterTabs, filterTabsMarkup(view));
  downloadsElements.downloadFilterOptions.dataset.catalogVersion = String(hfFilterCatalogVersion);
  setHTML(downloadsElements.downloadFilterOptions, filterGroupsMarkup(view));
  setHTML(downloadsElements.downloadFilterSummary, selectedFilterSummary(state.downloads.filters));
}

function filterPanelView(): FilterPanelView {
  const activeTab = state.downloads.filterTab;
  return {
    tabs: Object.keys(hfFilterCatalog),
    activeTab,
    groups: filterGroupsForTab(activeTab),
    query: downloadsElements.downloadFilterSearch.value.trim().toLocaleLowerCase(),
    selectedFilters: state.downloads.filters,
    expandedGroups: state.downloads.expandedFilterGroups
  };
}

function filterGroupsForTab(activeTab: string): FilterGroupView[] {
  const groups: FilterGroupView[] = [...(hfFilterCatalog[activeTab] || [])];
  if (activeTab === "main" && state.downloads.observedFilters.length > 0) {
    groups.push({id: "observed", label: "From current results", values: state.downloads.observedFilters});
  }
  return groups;
}
