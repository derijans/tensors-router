import { type SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import { badgeButton } from "../../ui/badge";
import { button } from "../../ui/buttons";
import { listOrFallback } from "../../ui/empty-state";
import { mutedText } from "../../ui/text";

export interface FilterGroupView {
  id: string;
  label: string;
  values: readonly string[];
}

export interface FilterPanelView {
  tabs: readonly string[];
  activeTab: string;
  groups: readonly FilterGroupView[];
  query: string;
  selectedFilters: readonly string[];
  expandedGroups: readonly string[];
}

const collapsedFilterGroupSize = 10;

export function filterTabsMarkup(view: FilterPanelView): SafeHTML {
  return html`${view.tabs.map(tab => html`<button type="button" data-download-filter-tab="${tab}"${tab === view.activeTab ? html` class="active"` : emptyHTML}>${tab}</button>`)}`;
}

export function filterGroupsMarkup(view: FilterPanelView): SafeHTML {
  const groups = view.groups.map(group => filterGroup(view, group)).filter(group => !group.isEmpty());
  return listOrFallback(groups, mutedText("No filters match."));
}

export function selectedFilterSummary(filters: readonly string[]): SafeHTML {
  if (filters.length === 0) {
    return mutedText("No metadata filters selected.");
  }
  return html`${filters.map(filter => badgeButton(`${filter} ×`, "accent", {"data-download-filter-clear": filter}))}${badgeButton("Clear all", "neutral", {"data-download-filter-clear-all": true})}`;
}

function filterGroup(view: FilterPanelView, group: FilterGroupView): SafeHTML {
  const matching = group.values.filter(value => !view.query || value.toLocaleLowerCase().includes(view.query));
  if (matching.length === 0) {
    return emptyHTML;
  }
  const expanded = view.query.length > 0 || view.expandedGroups.includes(`${view.activeTab}:${group.id}`);
  const visible = expanded ? matching : matching.slice(0, collapsedFilterGroupSize);
  const toggle = filterGroupToggle(group.id, matching.length - visible.length, expanded && matching.length > collapsedFilterGroupSize);
  return html`<section class="filter-group"><h4>${group.label}</h4><div class="filter-group-options">${visible.map(filter => filterChip(filter, view.selectedFilters.includes(filter)))}${toggle}</div></section>`;
}

function filterGroupToggle(groupID: string, hiddenCount: number, collapsible: boolean): SafeHTML {
  if (hiddenCount > 0) {
    return button(`+${hiddenCount} more`, {class: "filter-chip", "data-download-filter-group": groupID});
  }
  if (collapsible) {
    return button("Show less", {class: "filter-chip", "data-download-filter-group": groupID});
  }
  return emptyHTML;
}

function filterChip(filter: string, active: boolean): SafeHTML {
  return html`<button type="button" class="filter-chip${active ? " active" : ""}" data-download-filter="${filter}">${filterLabel(filter)}</button>`;
}

function filterLabel(value: string): string {
  return value.replace(/^(?:app|provider|dataset|library|language|license):/, "");
}
