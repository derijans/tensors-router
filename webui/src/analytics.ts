import { getAnalytics } from "./api";
import {
  analyticsModelChoices,
  analyticsNodeChoices,
  analyticsPeriods,
  analyticsSections,
  chartBars,
  formatCount,
  formatDecimal,
  formatDurationSeconds,
  normalizedAnalyticsQuery
} from "./analytics-data";
import { elements } from "./elements";
import { state } from "./state";
import type { AnalyticsModelUsage, AnalyticsNodeUsage, AnalyticsQuery, AnalyticsRecentEvent, AnalyticsSectionUsage, AnalyticsTimeline, SelectChoice } from "./types";
import { escapeAttribute, escapeHTML } from "./utils";

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

export function renderAnalytics(): void {
  renderAnalyticsControls();
  const data = state.analytics.data;
  if (state.analytics.error) {
    elements.analyticsStatus.innerHTML = `<div class="error-text">${escapeHTML(state.analytics.error)}</div>`;
  } else if (state.analytics.loading) {
    elements.analyticsStatus.innerHTML = `<div class="detail-empty">Loading analytics</div>`;
  } else if (!data?.enabled) {
    elements.analyticsStatus.innerHTML = `<div class="detail-empty">Analytics disabled</div>`;
  } else {
    elements.analyticsStatus.innerHTML = "";
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

function renderAnalyticsControls(): void {
  const query = normalizedAnalyticsQuery(state.analytics.query);
  elements.analyticsPeriodSelect.innerHTML = optionsHTML(analyticsPeriods, query.period);
  elements.analyticsNodeSelect.innerHTML = optionsHTML(choicesWithSelected(analyticsNodeChoices(state.inventory), query.node_id), query.node_id || "");
  elements.analyticsModelSelect.innerHTML = optionsHTML(choicesWithSelected(analyticsModelChoices(state.inventory), query.model_id), query.model_id || "");
  elements.analyticsSectionSelect.innerHTML = optionsHTML(analyticsSections, query.section || "");
}

function renderAnalyticsSummary(): void {
  const summary = state.analytics.data?.summary;
  if (!state.analytics.data?.enabled || !summary) {
    elements.analyticsSummary.innerHTML = "";
    return;
  }
  elements.analyticsSummary.innerHTML = [
    metricCard("Requests", formatCount(summary.request_count), `${formatCount(summary.success_count)} ok / ${formatCount(summary.failure_count)} failed`),
    metricCard("Tokens", formatCount(summary.total_tokens), `${formatCount(summary.input_tokens)} in / ${formatCount(summary.output_tokens)} out`),
    metricCard("Speed", `${formatDecimal(summary.average_tokens_per_second, 1)} tok/s`, `${formatDecimal(summary.average_duration_ms, 0)}ms avg`),
    metricCard("Images", formatCount(summary.image_count), "generated or returned"),
    metricCard("Audio", formatDurationSeconds(summary.audio_seconds), `${formatCount(summary.audio_tokens)} tokens`)
  ].join("");
}

function renderAnalyticsTimeline(): void {
  const timeline = state.analytics.data?.timeline ?? [];
  if (!state.analytics.data?.enabled || timeline.length === 0) {
    elements.analyticsTimeline.innerHTML = "";
    return;
  }
  const width = 720;
  const height = 180;
  const bars = chartBars(timeline, width, height);
  elements.analyticsTimeline.innerHTML = `
    <div class="analytics-chart-head">
      <strong>Timeline</strong>
      <span class="muted">${escapeHTML(state.analytics.data.granularity)}</span>
    </div>
    <svg class="analytics-chart" viewBox="0 0 ${width} ${height}" role="img" aria-label="Analytics timeline">
      ${bars.map((bar, index) => {
        const point = timeline[index];
        if (!point) {
          return "";
        }
        return `
        <rect class="analytics-bar" x="${bar.x.toFixed(2)}" y="${bar.y.toFixed(2)}" width="${bar.width.toFixed(2)}" height="${bar.height.toFixed(2)}">
          <title>${escapeHTML(formatBucket(point))}: ${formatCount(point.request_count)} requests</title>
        </rect>
      `;
      }).join("")}
    </svg>
  `;
}

function renderAnalyticsBreakdown(): void {
  const sections = state.analytics.data?.sections ?? [];
  if (!state.analytics.data?.enabled || sections.length === 0) {
    elements.analyticsSections.innerHTML = "";
    return;
  }
  const max = Math.max(...sections.map(section => section.request_count), 1);
  elements.analyticsSections.innerHTML = `
    <div class="analytics-chart-head">
      <strong>Sections</strong>
      <span class="muted">requests by lane</span>
    </div>
    <div class="analytics-section-bars">
      ${sections.map(section => sectionBar(section, max)).join("")}
    </div>
  `;
}

function renderAnalyticsTables(): void {
  const data = state.analytics.data;
  if (!data?.enabled) {
    elements.analyticsModelsTable.innerHTML = "";
    elements.analyticsNodesTable.innerHTML = "";
    elements.analyticsRecentTable.innerHTML = "";
    elements.analyticsNodeErrors.innerHTML = "";
    return;
  }
  elements.analyticsModelsTable.innerHTML = data.models.map(modelRow).join("");
  elements.analyticsNodesTable.innerHTML = data.nodes.map(nodeRow).join("");
  elements.analyticsRecentTable.innerHTML = data.recent.map(recentRow).join("");
  elements.analyticsNodeErrors.innerHTML = (data.node_errors ?? []).map(error => `
    <div class="error-text">${escapeHTML(error.node_id || error.node_url || "node")}: ${escapeHTML(error.error)}</div>
  `).join("");
}

function metricCard(label: string, value: string, detail: string): string {
  return `
    <article class="analytics-metric">
      <span>${escapeHTML(label)}</span>
      <strong>${escapeHTML(value)}</strong>
      <small>${escapeHTML(detail)}</small>
    </article>
  `;
}

function sectionBar(section: AnalyticsSectionUsage, max: number): string {
  const width = Math.max(1, Math.round((section.request_count / max) * 100));
  return `
    <div class="analytics-section-row">
      <span>${escapeHTML(sectionLabel(section.section))}</span>
      <svg viewBox="0 0 100 8" role="img" aria-label="${escapeAttribute(section.section)} requests">
        <rect class="analytics-bar-track" x="0" y="0" width="100" height="8"></rect>
        <rect class="analytics-bar" x="0" y="0" width="${width}" height="8"></rect>
      </svg>
      <strong>${formatCount(section.request_count)}</strong>
    </div>
  `;
}

function modelRow(model: AnalyticsModelUsage): string {
  return `
    <tr>
      <td>${escapeHTML(model.node_id)}</td>
      <td>${escapeHTML(model.model_id || "unknown")}</td>
      <td>${formatCount(model.request_count)}</td>
      <td>${formatCount(model.total_tokens)}</td>
      <td>${formatCount(model.image_count)}</td>
      <td>${formatDurationSeconds(model.audio_seconds)}</td>
    </tr>
  `;
}

function nodeRow(node: AnalyticsNodeUsage): string {
  return `
    <tr>
      <td>${escapeHTML(node.node_id)}</td>
      <td>${formatCount(node.request_count)}</td>
      <td>${formatCount(node.total_tokens)}</td>
      <td>${formatCount(node.image_count)}</td>
      <td>${formatDurationSeconds(node.audio_seconds)}</td>
    </tr>
  `;
}

function recentRow(event: AnalyticsRecentEvent): string {
  const media = event.section === "image"
    ? imageDetail(event)
    : event.section === "voice" || event.section === "music"
      ? audioDetail(event)
      : tokenDetail(event);
  return `
    <tr>
      <td>${escapeHTML(formatDate(event.finished_at))}</td>
      <td>${escapeHTML(event.node_id)}</td>
      <td>${escapeHTML(event.model_id || "unknown")}</td>
      <td>${escapeHTML(sectionLabel(event.section))}</td>
      <td>${escapeHTML(event.backend_mode || "")}</td>
      <td>${escapeHTML(event.success ? "ok" : String(event.status_code))}</td>
      <td>${escapeHTML(media)}</td>
    </tr>
  `;
}

function tokenDetail(event: AnalyticsRecentEvent): string {
  const speed = event.tokens_per_second ? ` / ${formatDecimal(event.tokens_per_second, 1)} tok/s` : "";
  return `${formatCount(event.input_tokens)} in / ${formatCount(event.output_tokens)} out${speed}`;
}

function imageDetail(event: AnalyticsRecentEvent): string {
  const resolution = event.image_width && event.image_height ? ` / ${event.image_width}x${event.image_height}` : "";
  const type = event.image_type ? `${event.image_type} / ` : "";
  return `${type}${formatCount(event.image_count)} images${resolution}`;
}

function audioDetail(event: AnalyticsRecentEvent): string {
  return `${formatDurationSeconds(event.audio_seconds)} / ${formatCount(event.audio_tokens)} tokens`;
}

function optionsHTML(options: SelectChoice[], selected: string): string {
  return options.map(option => `
    <option value="${escapeAttribute(option.value)}" ${option.value === selected ? "selected" : ""}>${escapeHTML(option.label)}</option>
  `).join("");
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
