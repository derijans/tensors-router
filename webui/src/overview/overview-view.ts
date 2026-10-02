import { SafeHTML, emptyHTML, html } from "../safe-html";
import { formatCount, formatDecimal } from "../analytics-data";
import { sparkline, stackedLaneChart } from "../chart-markup";
import { badge, laneAccent } from "../markup-primitives";
import type { LaneSeries } from "../lane-series";
import type { AnalyticsRecentEvent, Tone } from "../types";
import type { AttentionItem, OverviewKpi } from "./overview-data";

export function renderKpis(kpis: readonly OverviewKpi[]): SafeHTML {
  return html`${kpis.map((kpi, index) => html`
    <article class="kpi kpi-${index + 1}">
      <p class="kpi-label">${kpi.label}</p>
      <p class="kpi-value">${kpi.value}${kpi.unit ? html`<small>${kpi.unit}</small>` : emptyHTML}</p>
      <p class="kpi-detail tone-${kpi.detailTone}">${kpi.detail}</p>
      ${sparkline(kpi.trend, `${kpi.label} trend`)}
    </article>
  `)}`;
}

export function renderLaneChart(series: LaneSeries, granularity: string): SafeHTML {
  return stackedLaneChart(series, bucketStart => formatBucketTick(bucketStart, granularity), "Requests by lane over time");
}

export function formatBucketTick(bucketStart: number, granularity: string): string {
  const date = new Date(bucketStart);
  if (granularity === "hour") {
    return date.toLocaleTimeString("en-GB", {hour: "2-digit", minute: "2-digit"});
  }
  return date.toLocaleDateString("en-US", {month: "short", day: "numeric"});
}

export function renderAttention(items: readonly AttentionItem[]): SafeHTML {
  if (items.length === 0) {
    return html`<li class="attention-clear"><span class="status-dot tone-success">All clear</span><p class="muted">No failed loads, down nodes or build drift.</p></li>`;
  }
  return html`${items.map((item, index) => html`
    <li class="attention-item tone-${item.tone}">
      <span class="attention-icon"><svg class="icon"><use href="#${attentionIcon(item.tone)}"/></svg></span>
      <div class="attention-text">
        <strong>${item.title}</strong>
        <p>${item.detail}</p>
      </div>
      <button class="link-button" type="button" data-attention-index="${index}">${item.actionLabel}</button>
    </li>
  `)}`;
}

function attentionIcon(tone: Tone): string {
  return tone === "danger" ? "icon-errors" : "icon-nodes";
}

export function renderRecentRequests(events: readonly AnalyticsRecentEvent[]): SafeHTML {
  if (events.length === 0) {
    return html`<tr><td colspan="7" class="empty-state">No requests recorded in this period.</td></tr>`;
  }
  return html`${events.map(event => html`
    <tr>
      <td class="numeric-cell">${new Date(event.started_at).toLocaleTimeString("en-GB")}</td>
      <td><span class="lane-cell"><span class="lane-dot ${laneAccent(event.section)}"></span>${event.model_id || "unknown"}</span></td>
      <td>${event.node_id}</td>
      <td><code>${event.route}</code></td>
      <td>${badge(String(event.status_code), statusTone(event.status_code, event.success))}</td>
      <td class="numeric">${event.total_tokens ? formatCount(event.total_tokens) : "—"}</td>
      <td class="numeric">${formatDecimal(event.duration_ms / 1000, 1)} s</td>
    </tr>
  `)}`;
}

function statusTone(statusCode: number, success: boolean): Tone {
  if (success) {
    return "success";
  }
  return statusCode >= 500 || statusCode === 0 ? "danger" : "warning";
}
