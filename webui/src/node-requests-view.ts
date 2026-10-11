import { SafeHTML, emptyHTML, html } from "./safe-html";
import { badge, laneAccent } from "./markup-primitives";
import { groupRequestsByModel, type ModelRequestGroup } from "./node-requests-data";
import type { NodeRequest, NodeRequestState, Tone } from "./types";

const stateLabels: Record<NodeRequestState, string> = {
  running: "running",
  borrowed: "running for a peer",
  waiting_for_runtime: "waiting for runtime",
  queued: "queued",
  held: "held for helper",
  lent: "lent"
};

const stateTones: Record<NodeRequestState, Tone> = {
  running: "info",
  borrowed: "warning",
  waiting_for_runtime: "warning",
  queued: "neutral",
  held: "warning",
  lent: "success"
};

export function renderNodeRequests(requests: readonly NodeRequest[]): SafeHTML {
  const groups = groupRequestsByModel(requests);
  return html`
    <section class="node-requests" aria-label="Requests">
      <h4>Requests</h4>
      ${groups.length > 0 ? html`<ul>${groups.map(renderRequestGroup)}</ul>` : html`<p class="muted node-state-empty">No requests.</p>`}
    </section>
  `;
}

export function renderRequestStateBadges(group: ModelRequestGroup): SafeHTML {
  const states = [...group.counts.entries()].map(([state, count]) => badge(`${count} ${stateLabels[state] ?? state}`, stateTones[state] ?? "neutral"));
  return html`${states}`;
}

function renderRequestGroup(group: ModelRequestGroup): SafeHTML {
  return html`
    <li class="node-request-group">
      ${badge(group.lane, laneAccent(group.lane))}
      <span>${group.modelID}</span>
      ${renderRequestStateBadges(group)}
      <span class="muted">${formatAge(group.oldestAgeMS)}</span>
      ${group.helpers.length > 0 ? html`<span class="muted">→ ${group.helpers.join(", ")}</span>` : emptyHTML}
    </li>
  `;
}

function formatAge(milliseconds: number): string {
  return `${(Math.max(0, milliseconds) / 1000).toFixed(1)}s`;
}
