import { flushAnalytics, getAnalytics } from "../../../api/insights-api";
import { laneSeries } from "../../../domain/lane-series";
import { errorMessage } from "../../../lib/errors";
import { emptyHTML, html, setHTML } from "../../../lib/safe-html";
import { state } from "../../../state/app-state";
import type { AnalyticsQuery } from "../../../types";
import { emptyState } from "../../../ui/empty-state";
import { optionList } from "../../../ui/select";
import { errorText, nodeErrorLine, nodeErrorText } from "../../../ui/text";
import { insightsElements } from "../insights-elements";
import { analyticsModelChoices, analyticsNodeChoices, analyticsPeriods, analyticsSections, choicesWithSelected, isAnalyticsPeriod, normalizedAnalyticsQuery } from "./analytics-data";
import { renderVersionRows } from "./analytics-versions-view";
import { modelRow, nodeRow, recentRow, sectionTotalsMarkup, summaryCardsMarkup, timelineMarkup } from "./analytics-view";

type OptionalQueryFilter = "node_id" | "model_id" | "section";

const dataTables = [
  insightsElements.analyticsModelsTable,
  insightsElements.analyticsNodesTable,
  insightsElements.analyticsVersionsTable,
  insightsElements.analyticsRecentTable,
  insightsElements.analyticsNodeErrors
];

export async function loadAnalytics(): Promise<void> {
  state.analytics.loading = true;
  state.analytics.error = "";
  renderAnalytics();
  try {
    state.analytics.data = await getAnalytics(normalizedAnalyticsQuery(state.analytics.query));
  } catch (error) {
    state.analytics.error = errorMessage(error);
  } finally {
    state.analytics.loading = false;
    renderAnalytics();
  }
}

export async function flushAnalyticsToDisk(): Promise<void> {
  const response = await flushAnalytics();
  const failures = response.node_errors ?? [];
  if (failures.length > 0) {
    throw new Error(failures.map(nodeErrorLine).join("; "));
  }
  await loadAnalytics();
}

export function renderAnalytics(): void {
  renderAnalyticsControls();
  renderAnalyticsStatus();
  const data = state.analytics.data;
  if (!data?.enabled) {
    setHTML(insightsElements.analyticsSummary, emptyHTML);
    setHTML(insightsElements.analyticsTimeline, emptyHTML);
    setHTML(insightsElements.analyticsSections, emptyHTML);
    dataTables.forEach(table => setHTML(table, emptyHTML));
    return;
  }
  const sections = data.sections ?? [];
  setHTML(insightsElements.analyticsSummary, data.summary ? summaryCardsMarkup(data.summary) : emptyHTML);
  setHTML(insightsElements.analyticsTimeline, data.timeline.length
    ? timelineMarkup(laneSeries(data.timeline, {from: data.from, to: data.to, granularity: data.granularity}), data.granularity)
    : emptyHTML);
  setHTML(insightsElements.analyticsSections, sections.length ? sectionTotalsMarkup(sections) : emptyHTML);
  setHTML(insightsElements.analyticsModelsTable, html`${data.models.map(modelRow)}`);
  setHTML(insightsElements.analyticsNodesTable, html`${data.nodes.map(nodeRow)}`);
  setHTML(insightsElements.analyticsVersionsTable, renderVersionRows(data.router_versions ?? []));
  setHTML(insightsElements.analyticsRecentTable, html`${data.recent.map(event => recentRow(event, state.analytics.showDetails))}`);
  setHTML(insightsElements.analyticsNodeErrors, html`${(data.node_errors ?? []).map(failure => nodeErrorText(failure, "div"))}`);
}

export function updateAnalyticsPeriod(value: string): void {
  if (isAnalyticsPeriod(value)) {
    state.analytics.query.period = value;
  }
}

export function updateAnalyticsNode(value: string): void {
  setQueryFilter("node_id", value);
}

export function updateAnalyticsModel(value: string): void {
  setQueryFilter("model_id", value);
}

export function updateAnalyticsSection(value: string): void {
  setQueryFilter("section", value);
}

export function updateAnalyticsDetails(show: boolean): void {
  state.analytics.showDetails = show;
}

function setQueryFilter(filter: OptionalQueryFilter, value: string): void {
  const query: AnalyticsQuery = state.analytics.query;
  if (value) {
    query[filter] = value;
  } else {
    delete query[filter];
  }
}

function renderAnalyticsStatus(): void {
  if (state.analytics.error) {
    setHTML(insightsElements.analyticsStatus, errorText(state.analytics.error, "div"));
  } else if (state.analytics.loading) {
    setHTML(insightsElements.analyticsStatus, emptyState("Loading analytics"));
  } else if (!state.analytics.data?.enabled) {
    setHTML(insightsElements.analyticsStatus, emptyState("Analytics disabled"));
  } else {
    setHTML(insightsElements.analyticsStatus, emptyHTML);
  }
}

function renderAnalyticsControls(): void {
  insightsElements.analyticsDetailToggle.checked = state.analytics.showDetails;
  insightsElements.analyticsRecentDetailHeader.textContent = state.analytics.showDetails ? "Stream metrics" : "Metadata";
  const query = normalizedAnalyticsQuery(state.analytics.query);
  const filters = state.analytics.data?.filters;
  setHTML(insightsElements.analyticsPeriodSelect, optionList(analyticsPeriods, query.period));
  setHTML(insightsElements.analyticsNodeSelect, optionList(choicesWithSelected(analyticsNodeChoices(state.inventory, filters?.node_ids), query.node_id), query.node_id || ""));
  setHTML(insightsElements.analyticsModelSelect, optionList(choicesWithSelected(analyticsModelChoices(state.inventory, filters?.model_ids), query.model_id), query.model_id || ""));
  setHTML(insightsElements.analyticsSectionSelect, optionList(analyticsSections, query.section || ""));
}
