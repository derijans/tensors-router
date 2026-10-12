import { SafeHTML, emptyHTML, html } from "./safe-html";
import { badge, laneAccent } from "./markup-primitives";
import { requestsOldestFirst, type ModelRequestGroup } from "./node-requests-data";
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
  return html`
    <section class="node-requests" aria-label="Requests">
      <h4>Requests</h4>
      ${requests.length > 0 ? html`<ul>${requestsOldestFirst(requests).map(renderRequest)}</ul>` : html`<p class="muted node-state-empty">No requests.</p>`}
    </section>
  `;
}

function renderRequest(request: NodeRequest): SafeHTML {
  const helper = request.state === "lent" ? `${request.helper_node_id || "?"}/${request.helper_model_id || "?"}` : "";
  return html`
    <li class="node-request">
      ${badge(request.lane, laneAccent(request.lane))}
      <span>${request.model_id}</span>
      ${badge(stateLabels[request.state] ?? request.state, stateTones[request.state] ?? "neutral")}
      <span class="muted">${formatAge(request.age_ms)}</span>
      ${helper !== "" ? html`<span class="muted">→ ${helper}</span>` : emptyHTML}
    </li>
  `;
}

export function renderRequestStateBadges(group: ModelRequestGroup): SafeHTML {
  const states = [...group.counts.entries()].map(([state, count]) => badge(`${count} ${stateLabels[state] ?? state}`, stateTones[state] ?? "neutral"));
  return html`${states}`;
}

function formatAge(milliseconds: number): string {
  return `${(Math.max(0, milliseconds) / 1000).toFixed(1)}s`;
}
