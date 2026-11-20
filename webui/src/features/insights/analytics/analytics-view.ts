import { laneStackedSeries, type LaneSeries } from "../../../domain/lane-series";
import { laneAccent } from "../../../domain/lanes";
import { formatMegabytes } from "../../../lib/format-bytes";
import { formatCount, formatDecimal, formatPercent } from "../../../lib/format-number";
import { formatDurationSeconds, formatEpochTime } from "../../../lib/format-time";
import { type SafeHTML, html } from "../../../lib/safe-html";
import type { AnalyticsModelUsage, AnalyticsNodeUsage, AnalyticsRecentEvent, AnalyticsSectionUsage, AnalyticsSummary } from "../../../types";
import { badge } from "../../../ui/badge";
import { chartLegend } from "../../../ui/charts/chart-markup";
import { renderLaneChart } from "../../../ui/charts/lane-chart";
import { mutedText } from "../../../ui/text";
import { analyticsSectionLabel, promptSpeedPrefix } from "./analytics-data";
import { recentEventDetail } from "./analytics-event-detail-data";
import { cardHead } from "../../../ui/card-head";

export function summaryCardsMarkup(summary: AnalyticsSummary): SafeHTML {
  return html`${[
    metricCard("Requests", formatCount(summary.request_count), `${formatCount(summary.success_count)} ok / ${formatCount(summary.failure_count)} failed`),
    metricCard("Tokens", formatCount(summary.total_tokens), `${formatCount(summary.input_tokens)} in / ${formatCount(summary.output_tokens)} out`),
    metricCard("Speed", `${formatDecimal(summary.average_tokens_per_second, 1)} tok/s`, `${promptSpeedPrefix(summary.average_prompt_tokens_per_second)}${formatDecimal(summary.average_duration_ms, 0)}ms avg`),
    metricCard("Images", formatCount(summary.image_count), "generated or returned"),
    metricCard("Vectors", formatCount(summary.embedding_count), "embeddings returned"),
    metricCard("Audio", formatDurationSeconds(summary.audio_seconds), `${formatCount(summary.audio_tokens)} tokens`),
    metricCard("VRAM", formatMegabytes(summary.vram_peak_mb), `${formatPercent(summary.vram_peak_percent)} peak / ${formatMegabytes(summary.vram_total_mb)} total`),
    metricCard("Loads", formatCount(summary.load_count), `${formatDecimal(summary.average_load_duration_ms, 0)}ms avg / ${formatMegabytes(summary.model_vram_estimate_mb)} model`)
  ]}`;
}

export function timelineMarkup(series: LaneSeries, granularity: string): SafeHTML {
  const stacked = laneStackedSeries(series);
  return html`
    ${cardHead(html`
      <div>
        <h3>Requests by lane</h3>
        <ul class="lane-legend">${chartLegend(stacked)}</ul>
      </div>
    `, mutedText(html`per ${granularity}`))}
    ${renderLaneChart(stacked, granularity)}
  `;
}

export function sectionTotalsMarkup(sections: readonly AnalyticsSectionUsage[]): SafeHTML {
  const max = Math.max(...sections.map(section => section.request_count), 1);
  return html`
    ${cardHead(html`<h3>Lane totals</h3>`, mutedText("requests"))}
    <div class="section-bars">
      ${sections.map(section => sectionBar(section, max))}
    </div>
  `;
}

export function modelRow(model: AnalyticsModelUsage): SafeHTML {
  return html`
    <tr>
      <td>${model.node_id}</td>
      <td>${model.model_id || "unknown"}</td>
      <td>${formatCount(model.request_count)}</td>
      ${usageCells(model)}
    </tr>
  `;
}

export function nodeRow(node: AnalyticsNodeUsage): SafeHTML {
  return html`
    <tr>
      <td>${node.node_id}</td>
      <td>${formatCount(node.request_count)}</td>
      ${usageCells(node)}
    </tr>
  `;
}

export function recentRow(event: AnalyticsRecentEvent, showStreamMetrics: boolean): SafeHTML {
  return html`
    <tr>
      <td>${formatEpochTime(event.finished_at)}</td>
      <td>${event.node_id}</td>
      <td>${event.model_id || "unknown"}</td>
      <td>${analyticsSectionLabel(event.section)}</td>
      <td>${event.backend_mode || ""}</td>
      <td>${recentStatusBadge(event)}</td>
      <td>${recentEventDetail(event, showStreamMetrics)}</td>
    </tr>
  `;
}

function usageCells(usage: AnalyticsModelUsage | AnalyticsNodeUsage): SafeHTML {
  return html`
    <td>${formatCount(usage.load_count)}</td>
    <td>${formatMegabytes(usage.vram_peak_mb)} / ${formatPercent(usage.vram_peak_percent)}</td>
    <td>${formatCount(usage.total_tokens)}</td>
    <td>${formatCount(usage.image_count)}</td>
    <td>${formatCount(usage.embedding_count)}</td>
    <td>${formatDurationSeconds(usage.audio_seconds)}</td>
  `;
}

function metricCard(label: string, value: string, detail: string): SafeHTML {
  return html`
    <article class="metric">
      <span class="metric-label">${label}</span>
      <strong class="metric-value">${value}</strong>
      ${mutedText(detail, "small")}
    </article>
  `;
}

function sectionBar(section: AnalyticsSectionUsage, max: number): SafeHTML {
  const width = Math.max(1, Math.round((section.request_count / max) * 100));
  return html`
    <div class="section-bar ${laneAccent(section.section)}">
      <span>${analyticsSectionLabel(section.section)}</span>
      <svg viewBox="0 0 100 8" preserveAspectRatio="none" role="img" aria-label="${section.section} requests">
        <rect class="section-bar-track" x="0" y="0" width="100" height="8"></rect>
        <rect class="section-bar-fill" x="0" y="0" width="${width}" height="8"></rect>
      </svg>
      <strong>${formatCount(section.request_count)}</strong>
    </div>
  `;
}

function recentStatusBadge(event: AnalyticsRecentEvent): SafeHTML {
  if (event.success) {
    return badge("ok", "success");
  }
  const serverFailed = event.status_code >= 500 || event.status_code === 0;
  return badge(String(event.status_code), serverFailed ? "danger" : "warning");
}
