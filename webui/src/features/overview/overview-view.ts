import { SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import { sparkline } from "../../ui/charts/chart-markup";
import { badge, statusDot } from "../../ui/badge";
import type { AnalyticsRecentEvent, Tone } from "../../types";
import type { AttentionItem, OverviewKpi } from "./overview-data";
import { formatCount, formatDecimal } from "../../lib/format-number";
import { laneAccent } from "../../domain/lanes";
import { emptyTableRow } from "../../ui/empty-state";
import { mutedText } from "../../ui/text";
import { linkButton } from "../../ui/buttons";

export function renderKpis(kpis: readonly OverviewKpi[]): SafeHTML {
  return html`${kpis.map((kpi, index) => kpiCard(kpi, index))}`;
}

function kpiCard(kpi: OverviewKpi, index: number): SafeHTML {
  return html`
    <article class="kpi kpi-${index + 1}">
      <p class="kpi-label">${kpi.label}</p>
      <p class="kpi-value">${kpi.value}${kpiUnit(kpi.unit)}</p>
      <p class="kpi-detail tone-${kpi.detailTone}">${kpi.detail}</p>
      ${sparkline(kpi.trend, kpiTrendLabel(kpi))}
    </article>
  `;
}

function kpiUnit(unit: string): SafeHTML {
  return unit ? html`<small>${unit}</small>` : emptyHTML;
}

function kpiTrendLabel(kpi: OverviewKpi): string {
  return `${kpi.label} trend`;
}

export function renderAttention(items: readonly AttentionItem[]): SafeHTML {
  if (items.length === 0) {
    return html`<li class="attention-clear">${statusDot("All clear", "success")}${mutedText("No failed loads, down nodes or build drift.", "p")}</li>`;
  }
  return html`${items.map((item, index) => attentionEntry(item, index))}`;

}

function attentionEntry(item: AttentionItem, index: number): SafeHTML {
  return html`
    <li class="attention-item tone-${item.tone}">
      <span class="attention-icon"><svg class="icon"><use href="#${attentionIcon(item.tone)}"/></svg></span>
      <div class="attention-text">
        <strong>${item.title}</strong>
        <p>${item.detail}</p>
      </div>
      ${linkButton(item.actionLabel, {"data-attention-index": index})}
    </li>
  `;
}

function attentionIcon(tone: Tone): string {
  return tone === "danger" ? "icon-errors" : "icon-nodes";
}

export function renderRecentRequests(events: readonly AnalyticsRecentEvent[]): SafeHTML {
  if (events.length === 0) {
    return emptyTableRow("No requests recorded in this period.", 7);
  }
  return html`${events.map(recentRequestRow)}`;
}

function recentRequestRow(event: AnalyticsRecentEvent): SafeHTML {
  return html`
    <tr>
      <td class="numeric-cell">${new Date(event.started_at).toLocaleTimeString("en-GB")}</td>
      <td><span class="lane-cell"><span class="lane-dot ${laneAccent(event.section)}"></span>${event.model_id || "unknown"}</span></td>
      <td>${event.node_id}</td>
      <td><code>${event.route}</code></td>
      <td>${badge(String(event.status_code), statusTone(event.status_code, event.success))}</td>
      <td class="numeric">${event.total_tokens ? formatCount(event.total_tokens) : "—"}</td>
      <td class="numeric">${formatDecimal(event.duration_ms / 1000, 1)} s</td>
    </tr>
  `;
}


function statusTone(statusCode: number, success: boolean): Tone {
  if (success) {
    return "success";
  }
  return statusCode >= 500 || statusCode === 0 ? "danger" : "warning";
}
