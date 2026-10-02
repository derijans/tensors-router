import { getAnalytics, getNodeState } from "../api";
import { laneLegend } from "../chart-markup";
import { holdsOpenPopover } from "../dom";
import { elements } from "../elements";
import { laneSeries } from "../lane-series";
import { clusterBuildVersion, renderNodeCard } from "../node-card-view";
import { SafeHTML, html, setHTML } from "../safe-html";
import { state } from "../state";
import { errorMessage } from "../tasks";
import type { NodeState, OverviewPeriod } from "../types";
import { attentionItems, overviewKpis, recentRequests, type AttentionItem } from "./overview-data";
import { renderAttention, renderKpis, renderLaneChart, renderRecentRequests } from "./overview-view";

const analyticsRefreshMilliseconds = 30_000;
const nodeRefreshMilliseconds = 5_000;
const renderedMarkup = new WeakMap<Element, string>();

let analyticsTimer: number | null = null;
let nodeTimer: number | null = null;
let currentAttention: AttentionItem[] = [];

export async function loadOverview(): Promise<void> {
  await Promise.all([loadOverviewAnalytics(), loadNodeSnapshots()]);
  renderOverview();
}

export async function changeOverviewPeriod(period: OverviewPeriod): Promise<void> {
  state.overview.period = period;
  renderOverview();
  await loadOverviewAnalytics();
  renderOverview();
}

export async function refreshNodeSnapshot(nodeID: string): Promise<void> {
  state.nodeSnapshots[nodeID] = await getNodeState(nodeID);
  renderOverview();
}

export function attentionItemAt(index: number): AttentionItem | undefined {
  return currentAttention[index];
}

export function setOverviewActive(active: boolean): void {
  stopOverviewPolling();
  if (!active) {
    return;
  }
  renderOverview();
  analyticsTimer = window.setInterval(() => void loadOverviewAnalytics().then(renderOverview), analyticsRefreshMilliseconds);
  nodeTimer = window.setInterval(() => void loadNodeSnapshots().then(renderOverview), nodeRefreshMilliseconds);
}

export function stopOverviewPolling(): void {
  for (const timer of [analyticsTimer, nodeTimer]) {
    if (timer !== null) {
      window.clearInterval(timer);
    }
  }
  analyticsTimer = null;
  nodeTimer = null;
}

async function loadOverviewAnalytics(): Promise<void> {
  try {
    state.overview.analytics = await getAnalytics({period: state.overview.period});
    state.overview.error = "";
  } catch (error) {
    state.overview.error = errorMessage(error);
  }
}

async function loadNodeSnapshots(): Promise<void> {
  const nodes = (state.inventory?.nodes ?? []).filter(node => node.available);
  const results = await Promise.allSettled(nodes.map(node => getNodeState(node.node_id)));
  results.forEach((result, index) => {
    const nodeID = nodes[index]?.node_id;
    if (nodeID && result.status === "fulfilled") {
      state.nodeSnapshots[nodeID] = result.value;
    }
  });
}

export function renderOverview(): void {
  const analytics = state.overview.analytics;
  const nodes = state.inventory?.nodes ?? [];
  const snapshots = nodes.map(node => state.nodeSnapshots[node.node_id]).filter((snapshot): snapshot is NodeState => snapshot !== undefined);
  const loadErrorCount = state.loadErrors.enabled ? state.loadErrors.records.filter(record => record.severity === "error").length : 0;
  renderWhenChanged(elements.overviewKpis, renderKpis(overviewKpis(analytics, snapshots, loadErrorCount)));
  renderLaneSection();
  currentAttention = attentionItems({
    nodes,
    snapshots: state.nodeSnapshots,
    loadErrors: state.loadErrors.records,
    loadErrorsEnabled: state.loadErrors.enabled,
    failedCaptures: state.loadCaptures.attempts.filter(attempt => attempt.status === "failed"),
    analyticsNodeErrors: analytics?.node_errors ?? []
  });
  renderWhenChanged(elements.overviewAttention, renderAttention(currentAttention));
  elements.overviewAttentionCount.hidden = currentAttention.length === 0;
  elements.overviewAttentionCount.textContent = String(currentAttention.length);
  const buildVersion = clusterBuildVersion(nodes);
  const webuis = state.webuis.data?.data ?? [];
  renderWhenChanged(elements.overviewNodes, html`${nodes.map(node => renderNodeCard(node, {
    placement: "overview",
    expanded: false,
    clusterBuildVersion: buildVersion,
    snapshot: state.nodeSnapshots[node.node_id] ?? null,
    webuis
  }))}`);
  renderWhenChanged(elements.overviewRecent, renderRecentRequests(recentRequests(analytics)));
}

function renderLaneSection(): void {
  const analytics = state.overview.analytics;
  document.querySelectorAll<HTMLButtonElement>("[data-overview-period]").forEach(button => {
    button.classList.toggle("active", button.dataset.overviewPeriod === state.overview.period);
  });
  if (state.overview.error) {
    renderWhenChanged(elements.overviewLegend, html``);
    renderWhenChanged(elements.overviewChart, html`<div class="empty-state error-text">${state.overview.error}</div>`);
    return;
  }
  if (!analytics?.enabled) {
    renderWhenChanged(elements.overviewLegend, html``);
    renderWhenChanged(elements.overviewChart, html`<div class="empty-state">Analytics is off on this router, so there is no request history.</div>`);
    return;
  }
  const series = laneSeries(analytics.timeline, {from: analytics.from, to: analytics.to, granularity: analytics.granularity});
  renderWhenChanged(elements.overviewLegend, laneLegend(series));
  renderWhenChanged(elements.overviewChart, renderLaneChart(series, analytics.granularity));
}

function renderWhenChanged(element: Element, content: SafeHTML): void {
  const markup = SafeHTML.render(content);
  if (renderedMarkup.get(element) === markup || holdsOpenPopover(element)) {
    return;
  }
  renderedMarkup.set(element, markup);
  setHTML(element, content);
}
