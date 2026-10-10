import { SafeHTML, emptyHTML, html, setHTML } from "./safe-html";
import { flushAnalytics, getAnalytics } from "./api";
import {
  analyticsModelChoices,
  analyticsNodeChoices,
  analyticsPeriods,
  analyticsSections,
  formatCount,
  formatDecimal,
  formatDurationSeconds,
  formatMegabytes,
  formatPercent,
  normalizedAnalyticsQuery,
  promptSpeedPrefix
} from "./analytics-data";
import { failureDetail, queueWaitDetail } from "./analytics-failure-detail";
import { elements } from "./elements";
import { renderVersionRows } from "./analytics-versions-view";
import { laneLegend } from "./chart-markup";
import { laneSeries } from "./lane-series";
import { badge, laneAccent, optionElement } from "./markup-primitives";
import { renderLaneChart } from "./overview/overview-view";
import { state } from "./state";
import type { AnalyticsModelUsage, AnalyticsNodeError, AnalyticsNodeUsage, AnalyticsQuery, AnalyticsRecentEvent, AnalyticsSectionUsage, SelectChoice } from "./types";

export async function loadAnalytics(): Promise<void> {
  state.analytics.loading = true;
  state.analytics.error = "";
  renderAnalytics();
  try {
    state.analytics.data = await getAnalytics(normalizedAnalyticsQuery(state.analytics.query));
  } catch (error) {
    state.analytics.error = error instanceof Error ? error.message : String(error);
  } finally {
    state.analytics.loading = false;
    renderAnalytics();
  }
}

export async function flushAnalyticsToDisk(): Promise<void> {
  const response = await flushAnalytics();
  const failures = response.node_errors ?? [];
  if (failures.length > 0) {
    throw new Error(failures.map((failure) => `${failure.node_id || failure.node_url}: ${failure.error}`).join("; "));
  }
  await loadAnalytics();
}

export function renderAnalytics(): void {
  renderAnalyticsControls();
  const data = state.analytics.data;
  if (state.analytics.error) {
    setHTML(elements.analyticsStatus, html`<div class="error-text">${state.analytics.error}</div>`);
  } else if (state.analytics.loading) {
    setHTML(elements.analyticsStatus, html`<div class="detail-empty">Loading analytics</div>`);
  } else if (!data?.enabled) {
    setHTML(elements.analyticsStatus, html`<div class="detail-empty">Analytics disabled</div>`);
  } else {
    setHTML(elements.analyticsStatus, emptyHTML);
  }
  renderAnalyticsSummary();
  renderAnalyticsTimeline();
  renderAnalyticsBreakdown();
  renderAnalyticsTables();
}

export function updateAnalyticsPeriod(value: string): void {
  if (isAnalyticsPeriod(value)) {
    state.analytics.query.period = value;
  }
}

export function updateAnalyticsNode(value: string): void {
  if (value) {
    state.analytics.query.node_id = value;
  } else {
    delete state.analytics.query.node_id;
  }
}

export function updateAnalyticsModel(value: string): void {
  if (value) {
    state.analytics.query.model_id = value;
  } else {
    delete state.analytics.query.model_id;
  }
}

export function updateAnalyticsSection(value: string): void {
  if (value) {
    state.analytics.query.section = value;
  } else {
    delete state.analytics.query.section;
  }
}

export function updateAnalyticsDetails(show: boolean): void {
  state.analytics.showDetails = show;
}

function renderAnalyticsControls(): void {
  elements.analyticsDetailToggle.checked = state.analytics.showDetails;
  elements.analyticsRecentDetailHeader.textContent = state.analytics.showDetails ? "Stream metrics" : "Metadata";
  const query = normalizedAnalyticsQuery(state.analytics.query);
  const filters = state.analytics.data?.filters;
  setHTML(elements.analyticsPeriodSelect, optionsHTML(analyticsPeriods, query.period));
  setHTML(elements.analyticsNodeSelect, optionsHTML(choicesWithSelected(analyticsNodeChoices(state.inventory, filters?.node_ids), query.node_id), query.node_id || ""));
  setHTML(elements.analyticsModelSelect, optionsHTML(choicesWithSelected(analyticsModelChoices(state.inventory, filters?.model_ids), query.model_id), query.model_id || ""));
  setHTML(elements.analyticsSectionSelect, optionsHTML(analyticsSections, query.section || ""));
}

function renderAnalyticsSummary(): void {
  const summary = state.analytics.data?.summary;
  if (!state.analytics.data?.enabled || !summary) {
    setHTML(elements.analyticsSummary, emptyHTML);
    return;
  }
  setHTML(elements.analyticsSummary, html`${[
    metricCard("Requests", formatCount(summary.request_count), `${formatCount(summary.success_count)} ok / ${formatCount(summary.failure_count)} failed`),
    metricCard("Tokens", formatCount(summary.total_tokens), `${formatCount(summary.input_tokens)} in / ${formatCount(summary.output_tokens)} out`),
    metricCard("Speed", `${formatDecimal(summary.average_tokens_per_second, 1)} tok/s`, `${promptSpeedPrefix(summary.average_prompt_tokens_per_second)}${formatDecimal(summary.average_duration_ms, 0)}ms avg`),
    metricCard("Images", formatCount(summary.image_count), "generated or returned"),
    metricCard("Vectors", formatCount(summary.embedding_count), "embeddings returned"),
    metricCard("Audio", formatDurationSeconds(summary.audio_seconds), `${formatCount(summary.audio_tokens)} tokens`),
    metricCard("VRAM", formatMegabytes(summary.vram_peak_mb), `${formatPercent(summary.vram_peak_percent)} peak / ${formatMegabytes(summary.vram_total_mb)} total`),
    metricCard("Loads", formatCount(summary.load_count), `${formatDecimal(summary.average_load_duration_ms, 0)}ms avg / ${formatMegabytes(summary.model_vram_estimate_mb)} model`)
  ]}`);
}

function renderAnalyticsTimeline(): void {
  const data = state.analytics.data;
  if (!data?.enabled || data.timeline.length === 0) {
    setHTML(elements.analyticsTimeline, emptyHTML);
    return;
  }
  const series = laneSeries(data.timeline, {from: data.from, to: data.to, granularity: data.granularity});
  setHTML(elements.analyticsTimeline, html`
    <header class="card-head">
      <div>
        <h3>Requests by lane</h3>
        <ul class="lane-legend">${laneLegend(series)}</ul>
      </div>
      <span class="muted">per ${data.granularity}</span>
    </header>
    ${renderLaneChart(series, data.granularity)}
  `);
}

function renderAnalyticsBreakdown(): void {
  const sections = state.analytics.data?.sections ?? [];
  if (!state.analytics.data?.enabled || sections.length === 0) {
    setHTML(elements.analyticsSections, emptyHTML);
    return;
  }
  const max = Math.max(...sections.map(section => section.request_count), 1);
  setHTML(elements.analyticsSections, html`
    <header class="card-head">
      <h3>Lane totals</h3>
      <span class="muted">requests</span>
    </header>
    <div class="section-bars">
      ${sections.map(section => sectionBar(section, max))}
    </div>
  `);
}

function renderAnalyticsTables(): void {
  const data = state.analytics.data;
  if (!data?.enabled) {
    setHTML(elements.analyticsModelsTable, emptyHTML);
    setHTML(elements.analyticsNodesTable, emptyHTML);
    setHTML(elements.analyticsVersionsTable, emptyHTML);
    setHTML(elements.analyticsRecentTable, emptyHTML);
    setHTML(elements.analyticsNodeErrors, emptyHTML);
    return;
  }
  setHTML(elements.analyticsModelsTable, html`${data.models.map(modelRow)}`);
  setHTML(elements.analyticsNodesTable, html`${data.nodes.map(nodeRow)}`);
  setHTML(elements.analyticsVersionsTable, renderVersionRows(data.router_versions ?? []));
  setHTML(elements.analyticsRecentTable, html`${data.recent.map(recentRow)}`);
  setHTML(elements.analyticsNodeErrors, html`${(data.node_errors ?? []).map(nodeErrorLine)}`);
}

function nodeErrorLine(error: AnalyticsNodeError): SafeHTML {
  return html`<div class="error-text">${error.node_id || error.node_url || "node"}: ${error.error}</div>`;
}

function metricCard(label: string, value: string, detail: string): SafeHTML {
  return html`
    <article class="metric">
      <span class="metric-label">${label}</span>
      <strong class="metric-value">${value}</strong>
      <small class="muted">${detail}</small>
    </article>
  `;
}

function sectionBar(section: AnalyticsSectionUsage, max: number): SafeHTML {
  const width = Math.max(1, Math.round((section.request_count / max) * 100));
  return html`
    <div class="section-bar ${laneAccent(section.section)}">
      <span>${sectionLabel(section.section)}</span>
      <svg viewBox="0 0 100 8" preserveAspectRatio="none" role="img" aria-label="${section.section} requests">
        <rect class="section-bar-track" x="0" y="0" width="100" height="8"></rect>
        <rect class="section-bar-fill" x="0" y="0" width="${width}" height="8"></rect>
      </svg>
      <strong>${formatCount(section.request_count)}</strong>
    </div>
  `;
}

function modelRow(model: AnalyticsModelUsage): SafeHTML {
  return html`
    <tr>
      <td>${model.node_id}</td>
      <td>${model.model_id || "unknown"}</td>
      <td>${formatCount(model.request_count)}</td>
      <td>${formatCount(model.load_count)}</td>
      <td>${formatMegabytes(model.vram_peak_mb)} / ${formatPercent(model.vram_peak_percent)}</td>
      <td>${formatCount(model.total_tokens)}</td>
      <td>${formatCount(model.image_count)}</td>
      <td>${formatCount(model.embedding_count)}</td>
      <td>${formatDurationSeconds(model.audio_seconds)}</td>
    </tr>
  `;
}

function nodeRow(node: AnalyticsNodeUsage): SafeHTML {
  return html`
    <tr>
      <td>${node.node_id}</td>
      <td>${formatCount(node.request_count)}</td>
      <td>${formatCount(node.load_count)}</td>
      <td>${formatMegabytes(node.vram_peak_mb)} / ${formatPercent(node.vram_peak_percent)}</td>
      <td>${formatCount(node.total_tokens)}</td>
      <td>${formatCount(node.image_count)}</td>
      <td>${formatCount(node.embedding_count)}</td>
      <td>${formatDurationSeconds(node.audio_seconds)}</td>
    </tr>
  `;
}

function recentRow(event: AnalyticsRecentEvent): SafeHTML {
  return html`
    <tr>
      <td>${formatDate(event.finished_at)}</td>
      <td>${event.node_id}</td>
      <td>${event.model_id || "unknown"}</td>
      <td>${sectionLabel(event.section)}</td>
      <td>${event.backend_mode || ""}</td>
      <td>${recentStatusBadge(event)}</td>
      <td>${recentEventDetail(event)}</td>
    </tr>
  `;
}

function recentStatusBadge(event: AnalyticsRecentEvent): SafeHTML {
  if (event.success) {
    return badge("ok", "success");
  }
  const serverFailed = event.status_code >= 500 || event.status_code === 0;
  return badge(String(event.status_code), serverFailed ? "danger" : "warning");
}

function recentEventDetail(event: AnalyticsRecentEvent): string {
  const failure = failureDetail(event);
  if (failure) {
    return failure;
  }
  if (state.analytics.showDetails) {
    return streamDetail(event);
  }
  if (event.event_type === "model_load") {
    return loadDetail(event);
  }
  switch (event.section) {
    case "image":
      return imageDetail(event);
    case "embed":
      return embeddingDetail(event);
    case "voice":
    case "music":
      return audioDetail(event);
    default:
      return tokenDetail(event);
  }
}

function streamDetail(event: AnalyticsRecentEvent): string {
  if (event.event_type === "model_load") {
    return loadDetail(event);
  }
  const parts: string[] = [];
  if (event.section === "llm") {
    parts.push(tokenCounts(event));
  }
  const queued = queueWaitDetail(event);
  if (queued) {
    parts.push(queued);
  }
  if (event.ttft_ms) {
    parts.push(`TTFT ${formatDecimal(event.ttft_ms, 0)}ms`);
  }
  if (event.decode_ms) {
    parts.push(`decode ${formatDecimal(event.decode_ms, 0)}ms`);
    const intervals = (event.output_tokens ?? 0) - 1;
    if (intervals > 0) {
      parts.push(`${formatDecimal((intervals * 1000) / event.decode_ms, 1)} tok/s decode`);
    }
  }
  if (event.max_gap_ms) {
    parts.push(`max gap ${formatDecimal(event.max_gap_ms, 0)}ms`);
  }
  if (event.finish_reason) {
    parts.push(event.finish_reason);
  }
  if (event.aborted) {
    parts.push("aborted");
  }
  if (parts.length === 0) {
    return "no stream metrics";
  }
  return parts.join(" / ");
}

function tokenDetail(event: AnalyticsRecentEvent): string {
  const speed = event.tokens_per_second ? ` / ${formatDecimal(event.tokens_per_second, 1)} tok/s` : "";
  const promptSpeed = event.prompt_tokens_per_second ? ` / ${formatDecimal(event.prompt_tokens_per_second, 0)} tok/s prompt` : "";
  return `${tokenCounts(event)}${speed}${promptSpeed}${workVRAMDetail(event)}`;
}

function tokenCounts(event: AnalyticsRecentEvent): string {
  return `${formatCount(event.input_tokens)} in / ${formatCount(event.output_tokens)} out`;
}

function embeddingDetail(event: AnalyticsRecentEvent): string {
  return `${formatCount(event.input_tokens)} in / ${formatCount(event.embedding_count)} vectors${workVRAMDetail(event)}`;
}

function imageDetail(event: AnalyticsRecentEvent): string {
  const resolution = event.image_width && event.image_height ? ` / ${event.image_width}x${event.image_height}` : "";
  const type = event.image_type ? `${event.image_type} / ` : "";
  return `${type}${formatCount(event.image_count)} images${resolution}${workVRAMDetail(event)}`;
}

function audioDetail(event: AnalyticsRecentEvent): string {
  const metadata = [event.audio_language, event.audio_task].filter(Boolean).join(" / ");
  const prefix = metadata ? `${metadata} / ` : "";
  return `${prefix}${formatDurationSeconds(event.audio_seconds)} / ${formatCount(event.audio_tokens)} tokens${workVRAMDetail(event)}`;
}

function loadDetail(event: AnalyticsRecentEvent): string {
  const config = event.config_filename ? `${event.config_filename} / ` : "";
  return `${config}${formatDecimal(event.duration_ms, 0)}ms / ${formatMegabytes(event.load_vram_before_mb)} -> ${formatMegabytes(event.load_vram_after_mb)} / +${formatMegabytes(event.load_vram_delta_mb)}`;
}

function workVRAMDetail(event: AnalyticsRecentEvent): string {
  if (!event.work_vram_max_mb && !event.model_vram_estimate_mb) {
    return "";
  }
  return ` / VRAM ${formatMegabytes(event.work_vram_max_mb)} (${formatPercent(event.vram_peak_percent)}) / model ${formatMegabytes(event.model_vram_estimate_mb)}`;
}

function optionsHTML(options: SelectChoice[], selected: string): SafeHTML {
  return html`${options.map(option => optionElement(option.value, option.label, option.value === selected))}`;
}

function choicesWithSelected(options: SelectChoice[], selected: string | undefined): SelectChoice[] {
  if (!selected || options.some(option => option.value === selected)) {
    return options;
  }
  return [...options, {value: selected, label: selected}];
}

function isAnalyticsPeriod(value: string): value is AnalyticsQuery["period"] {
  return value === "24h" || value === "7d" || value === "30d" || value === "90d" || value === "all";
}

function sectionLabel(section: string): string {
  return analyticsSections.find(option => option.value === section)?.label ?? section;
}

function formatDate(value: number): string {
  if (!value) {
    return "never";
  }
  return new Date(value).toLocaleString();
}
