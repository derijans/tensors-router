import { SafeHTML, emptyHTML, html } from "./safe-html";
import { type LendingDecisionGroup, type LendingMeasureField, lendingMeasureFields } from "./lending-log-collapse";
import type { LendingDecision, Tone } from "./types";
import { badge, fact } from "./markup-primitives";

const measureLabels: Record<LendingMeasureField, string> = {
  pending_count: "Pending (withdrawable)",
  backlog_count: "Backlog",
  keep_ms: "Owner drain alone (ms)",
  switch_ms: "Helper load (ms)",
  service_ms: "Helper per job (ms)",
  owner_job_ms: "Owner per job (ms)",
  helper_idle_ms: "Helper idle (ms)",
  slots: "Slots",
  lent_out: "Lent out",
  borrowed_ahead: "Borrowed ahead",
  wait_ms: "Waited (ms)"
};

export function renderDecisionRows(groups: LendingDecisionGroup[], selectedID: string): SafeHTML {
  return html`${groups.map(group => {
    const decision = group.latest;
    const selected = group.id === selectedID ? html` class="selected"` : emptyHTML;
    return html`<tr data-lending-group="${group.id}"${selected}>
      <td class="lending-nowrap">${formatSpan(group)}</td>
      <td class="lending-nowrap">${group.count > 1 ? `×${group.count}` : ""}</td>
      <td class="lending-nowrap">${decision.node_id}</td>
      <td class="lending-nowrap">${decision.kind}</td>
      <td class="lending-nowrap">${decision.trigger ?? ""}</td>
      <td class="lending-nowrap">${decision.lane}</td>
      <td class="lending-endpoint">${endpointCell(decision.owner_node_id, decision.owner_model_id)}</td>
      <td class="lending-endpoint">${endpointCell(decision.helper_node_id, decision.helper_model_id)}</td>
      <td>${badge(decision.outcome, outcomeTone(decision.outcome))}</td>
      <td>${decision.reason ?? ""}</td>
    </tr>`;
  })}`;
}

export function renderDecisionDetail(group: LendingDecisionGroup | undefined): SafeHTML {
  if (!group) {
    return emptyHTML;
  }
  const decision = group.latest;
  const facts: [string, string][] = [
    ["Seen", group.count > 1 ? `${group.count} times, ${formatTime(group.earliest.recorded_at)} – ${formatTime(decision.recorded_at)}` : formatTime(decision.recorded_at)],
    ["Node", decision.node_id],
    ["Router version", decision.router_version ?? ""],
    ["Owner", endpoint(decision.owner_node_id, decision.owner_model_id)],
    ["Helper", endpoint(decision.helper_node_id, decision.helper_model_id)],
    ["Outcome", [decision.outcome, decision.reason].filter(Boolean).join(" / ")],
    ["Service estimate from", decision.service_source ?? ""],
    ...lendingMeasureFields.map((field): [string, string] => [measureLabels[field], formatRange(group, field)])
  ];
  return html`
    <h3>${decision.kind} ${decision.outcome}</h3>
    <dl class="fact-grid">
      ${facts.filter(([, value]) => value !== "").map(([label, value]) => fact(label, value))}
    </dl>
  `;
}

export function decisionMatchesSearch(decision: LendingDecision, search: string): boolean {
  const needle = search.trim().toLowerCase();
  if (!needle) {
    return true;
  }
  return [decision.node_id, decision.owner_node_id, decision.owner_model_id, decision.helper_node_id, decision.helper_model_id]
    .some(value => (value ?? "").toLowerCase().includes(needle));
}

function formatRange(group: LendingDecisionGroup, field: LendingMeasureField): string {
  const range = group.ranges[field];
  if (!range || (range.min === 0 && range.max === 0)) {
    return "";
  }
  return range.min === range.max ? formatNumber(range.min) : `${formatNumber(range.min)} – ${formatNumber(range.max)}`;
}

function formatNumber(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

function formatSpan(group: LendingDecisionGroup): string {
  return group.count > 1 ? `${formatTime(group.earliest.recorded_at)} – ${formatTime(group.latest.recorded_at)}` : formatTime(group.latest.recorded_at);
}

function formatTime(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleTimeString();
}

function endpointCell(nodeID: string | undefined, modelID: string | undefined): SafeHTML {
  if (!nodeID && !modelID) {
    return emptyHTML;
  }
  return html`<span class="lending-endpoint-node">${nodeID ?? "?"}</span><span class="muted">${modelID ?? "?"}</span>`;
}

function endpoint(nodeID: string | undefined, modelID: string | undefined): string {
  return nodeID || modelID ? `${nodeID ?? "?"}/${modelID ?? "?"}` : "";
}

function outcomeTone(outcome: string): Tone {
  switch (outcome) {
    case "granted":
    case "lent":
    case "lease_updated":
      return "success";
    case "probe":
      return "info";
    case "skipped":
    case "relay_refused":
    case "lease_refused":
    case "borrowed_returned":
      return "warning";
    case "returned":
    case "lease_cleared":
      return "neutral";
    default:
      return "accent";
  }
}
