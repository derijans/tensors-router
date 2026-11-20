import { type SafeHTML, html } from "../../../lib/safe-html";
import type { BenchmarkSection, BenchmarkSummary } from "../../../types";
import { badge, badgeRow } from "../../../ui/badge";
import { fact, factGrid } from "../../../ui/fact-grid";
import { labelledToggle } from "../../../ui/form";
import { errorText, mutedText } from "../../../ui/text";
import { formatEpochTime } from "../../../lib/format-time";
import { benchmarkStatusTone, formatMetricValue } from "./benchmark-data";

export function sectionToggle(section: BenchmarkSection, selected: boolean, locked: boolean): SafeHTML {
  return html`
    ${labelledToggle({
      value: section,
      "data-operation-group": "benchmark",
      "data-benchmark-section": section,
      disabled: locked,
      "data-unavailable": locked
    }, selected, section)}
  `;
}

export function historyRow(summary: BenchmarkSummary): SafeHTML {
  return html`
    <article class="benchmark-row">
      <div>
        <strong>${summary.section}</strong>
        ${mutedText(html`${formatEpochTime(summary.finished_at)} · ${summary.duration_ms || 0} ms`, "div")}
      </div>
      ${badge(summary.status, benchmarkStatusTone(summary.status))}
      ${badgeRow(optionChanges(summary))}
    </article>
  `;
}

export function summaryCard(title: string, summary: BenchmarkSummary): SafeHTML {
  return html`
    <article class="card benchmark-card">
      <header class="benchmark-card-head">
        <h3>${title}</h3>
        ${badge(summary.status, benchmarkStatusTone(summary.status))}
      </header>
      ${mutedText(html`${summary.duration_ms || 0} ms · ${formatEpochTime(summary.finished_at)}`, "p")}
      ${summary.error ? errorText(summary.error) : ""}
      ${factGrid((summary.metrics ?? []).map(metric => fact(metric.name, formatMetricValue(metric))))}
    </article>
  `;
}

function optionChanges(summary: BenchmarkSummary): SafeHTML {
  const changes = summary.option_changes ?? [];
  if (changes.length === 0) {
    return mutedText("no option changes");
  }
  return html`${changes.map(optionChangeBadge)}`;
}

function optionChangeBadge(change: NonNullable<BenchmarkSummary["option_changes"]>[number]): SafeHTML {
  return badge(`${change.key} ${change.kind}`, "warning");
}
