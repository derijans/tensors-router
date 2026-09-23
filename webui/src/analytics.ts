import { SafeHTML, emptyHTML, html, setHTML } from "./safe-html";
import { flushAnalytics, getAnalytics } from "./api";
import {
  analyticsModelChoices,
  analyticsNodeChoices,
  analyticsPeriods,
  analyticsSections,
  chartPoints,
  formatCount,
  formatDecimal,
  formatDurationSeconds,
  formatMegabytes,
  formatPercent,
  normalizedAnalyticsQuery
} from "./analytics-data";
import { elements } from "./elements";
import { state } from "./state";
import type { AnalyticsModelUsage, AnalyticsNodeUsage, AnalyticsQuery, AnalyticsRecentEvent, AnalyticsSectionUsage, AnalyticsTimeline, SelectChoice } from "./types";

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
    metricCard("Speed", `${formatDecimal(summary.average_tokens_per_second, 1)} tok/s`, `${formatDecimal(summary.average_duration_ms, 0)}ms avg`),
    metricCard("Images", formatCount(summary.image_count), "generated or returned"),
    metricCard("Vectors", formatCount(summary.embedding_count), "embeddings returned"),
    metricCard("Audio", formatDurationSeconds(summary.audio_seconds), `${formatCount(summary.audio_tokens)} tokens`),
    metricCard("VRAM", formatMegabytes(summary.vram_peak_mb), `${formatPercent(summary.vram_peak_percent)} peak / ${formatMegabytes(summary.vram_total_mb)} total`),
    metricCard("Loads", formatCount(summary.load_count), `${formatDecimal(summary.average_load_duration_ms, 0)}ms avg / ${formatMegabytes(summary.model_vram_estimate_mb)} model`)
  ]}`);
}

function renderAnalyticsTimeline(): void {
  const timeline = state.analytics.data?.timeline ?? [];
  if (!state.analytics.data?.enabled || timeline.length === 0) {
    setHTML(elements.analyticsTimeline, emptyHTML);
    return;
  }
  const width = 720;
  const plotHeight = 170;
  const height = 220;
  const series = chartPoints(timeline, width, plotHeight);
  setHTML(elements.analyticsTimeline, html`
    <div class="analytics-chart-head">
      <strong>Timeline</strong>
      <span class="muted">${state.analytics.data.granularity}</span>
    </div>
    <svg class="analytics-chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="Analytics timeline">
      <path class="analytics-line" d="${series.linePath}"></path>
      ${series.points.map((chartPoint, index) => {
        const point = timeline[index];
        if (!point) {
          return "";
        }
        return html`
        <circle class="analytics-point" cx="${chartPoint.x.toFixed(2)}" cy="${chartPoint.y.toFixed(2)}" r="${chartPoint.radius.toFixed(2)}">
          <title>${formatBucket(point)}: ${formatCount(point.request_count)} requests</title>
        </circle>
      `;
      })}
      <line class="analytics-axis" x1="4" y1="${plotHeight + 10}" x2="${width - 4}" y2="${plotHeight + 10}"></line>
      ${series.ticks.map(tick => html`
        <g class="analytics-tick">
          <line class="analytics-axis" x1="${tick.x.toFixed(2)}" y1="${plotHeight + 5}" x2="${tick.x.toFixed(2)}" y2="${plotHeight + 15}"></line>
          <text class="analytics-tick-label" x="${tick.x.toFixed(2)}" y="${plotHeight + 34}">${tick.label}</text>
        </g>
      `)}
    </svg>
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
    <div class="analytics-chart-head">
      <strong>Sections</strong>
      <span class="muted">requests by lane</span>
    </div>
    <div class="analytics-section-bars">
      ${sections.map(section => sectionBar(section, max))}
    </div>
  `);
}

function renderAnalyticsTables(): void {
  const data = state.analytics.data;
  if (!data?.enabled) {
    setHTML(elements.analyticsModelsTable, emptyHTML);
    setHTML(elements.analyticsNodesTable, emptyHTML);
    setHTML(elements.analyticsRecentTable, emptyHTML);
    setHTML(elements.analyticsNodeErrors, emptyHTML);
    return;
  }
  setHTML(elements.analyticsModelsTable, html`${data.models.map(modelRow)}`);
  setHTML(elements.analyticsNodesTable, html`${data.nodes.map(nodeRow)}`);
  setHTML(elements.analyticsRecentTable, html`${data.recent.map(recentRow)}`);
  setHTML(elements.analyticsNodeErrors, html`${(data.node_errors ?? []).map(error => html`
    <div class="error-text">${error.node_id || error.node_url || "node"}: ${error.error}</div>
  `)}`);
}

function metricCard(label: string, value: string, detail: string): SafeHTML {
  return html`
    <article class="analytics-metric">
      <span>${label}</span>
      <strong>${value}</strong>
      <small>${detail}</small>
    </article>
  `;
}

function sectionBar(section: AnalyticsSectionUsage, max: number): SafeHTML {
  const width = Math.max(1, Math.round((section.request_count / max) * 100));
  return html`
    <div class="analytics-section-row">
      <span>${sectionLabel(section.section)}</span>
      <svg viewBox="0 0 100 8" role="img" aria-label="${section.section} requests">
        <rect class="analytics-bar-track" x="0" y="0" width="100" height="8"></rect>
        <rect class="analytics-bar" x="0" y="0" width="${width}" height="8"></rect>
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
  const media = state.analytics.showDetails
    ? streamDetail(event)
    : event.event_type === "model_load"
    ? loadDetail(event)
    : event.section === "image"
    ? imageDetail(event)
    : event.section === "embed"
    ? embeddingDetail(event)
    : event.section === "voice" || event.section === "music"
      ? audioDetail(event)
      : tokenDetail(event);
  return html`
    <tr>
      <td>${formatDate(event.finished_at)}</td>
      <td>${event.node_id}</td>
      <td>${event.model_id || "unknown"}</td>
      <td>${sectionLabel(event.section)}</td>
      <td>${event.backend_mode || ""}</td>
      <td>${event.success ? "ok" : String(event.status_code)}</td>
      <td>${media}</td>
    </tr>
  `;
}

function streamDetail(event: AnalyticsRecentEvent): string {
  if (event.event_type === "model_load") {
    return loadDetail(event);
  }
  const parts: string[] = [];
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
  return `${formatCount(event.input_tokens)} in / ${formatCount(event.output_tokens)} out${speed}${workVRAMDetail(event)}`;
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
  return html`${options.map(option => html`
    <option value="${option.value}" ${option.value === selected ? "selected" : ""}>${option.label}</option>
  `)}`;
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

function formatBucket(point: AnalyticsTimeline): string {
  return formatDate(point.bucket_start);
}

function formatDate(value: number): string {
  if (!value) {
    return "never";
  }
  return new Date(value).toLocaleString();
}
