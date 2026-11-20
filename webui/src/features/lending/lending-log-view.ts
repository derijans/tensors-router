import { SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import { type LendingDecisionGroup, type LendingMeasureField, lendingMeasureFields } from "./lending-log-collapse-data";
import type { LendingDecision, Tone } from "../../types";
import { badge } from "../../ui/badge";
import { formatClockTime } from "../../lib/format-time";
import { formatIntegerOrFixed } from "../../lib/format-number";
import { presentFactGrid } from "../../ui/fact-grid";
import { mutedText } from "../../ui/text";
import { lookupOr } from "../../lib/collections";

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
    ["Seen", group.count > 1 ? `${group.count} times, ${formatClockTime(group.earliest.recorded_at)} – ${formatClockTime(decision.recorded_at)}` : formatClockTime(decision.recorded_at)],
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
    ${presentFactGrid(facts)}
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
  return range.min === range.max ? formatIntegerOrFixed(range.min, 1) : `${formatIntegerOrFixed(range.min, 1)} – ${formatIntegerOrFixed(range.max, 1)}`;
}

function formatSpan(group: LendingDecisionGroup): string {
  return group.count > 1 ? `${formatClockTime(group.earliest.recorded_at)} – ${formatClockTime(group.latest.recorded_at)}` : formatClockTime(group.latest.recorded_at);
}

function endpointCell(nodeID: string | undefined, modelID: string | undefined): SafeHTML {
  if (!nodeID && !modelID) {
    return emptyHTML;
  }
  return html`<span class="lending-endpoint-node">${nodeID ?? "?"}</span>${mutedText(modelID ?? "?")}`;
}

function endpoint(nodeID: string | undefined, modelID: string | undefined): string {
  return nodeID || modelID ? `${nodeID ?? "?"}/${modelID ?? "?"}` : "";
}

const outcomeTones: Record<string, Tone> = {
  granted: "success",
  lent: "success",
  lease_updated: "success",
  probe: "info",
  skipped: "warning",
  relay_refused: "warning",
  lease_refused: "warning",
  borrowed_returned: "warning",
  returned: "neutral",
  lease_cleared: "neutral"
};

function outcomeTone(outcome: string): Tone {
  return lookupOr(outcomeTones, outcome, "accent");
}
