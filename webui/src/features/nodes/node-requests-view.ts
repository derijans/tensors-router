import { SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import { badge } from "../../ui/badge";
import { requestsOldestFirst, type ModelRequestGroup } from "./node-requests-data";
import type { NodeRequest, NodeRequestState, Tone } from "../../types";
import { formatElapsedSeconds } from "../../lib/format-time";
import { laneAccent } from "../../domain/lanes";
import { mutedText } from "../../ui/text";

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
      ${requests.length > 0 ? html`<ul>${requestsOldestFirst(requests).map(renderRequest)}</ul>` : mutedText("No requests.", "p", "node-state-empty")}
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
      ${mutedText(formatElapsedSeconds(request.age_ms))}
      ${helper !== "" ? mutedText(html`→ ${helper}`) : emptyHTML}
    </li>
  `;
}

export function renderRequestStateBadges(group: ModelRequestGroup): SafeHTML {
  const states = [...group.counts.entries()].map(([state, count]) => badge(`${count} ${stateLabels[state] ?? state}`, stateTones[state] ?? "neutral"));
  return html`${states}`;
}
