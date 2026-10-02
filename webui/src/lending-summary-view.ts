import { SafeHTML, html } from "./safe-html";
import type { LendingLease, LendingNodeSummary, LendingSummaryResponse } from "./types";
import { badge, laneAccent } from "./markup-primitives";

export function renderLendingSummary(summary: LendingSummaryResponse, masterFingerprint: string, now: Date): SafeHTML {
  const leases = summary.leases ?? [];
  return html`
    <div class="table-wrap">
      <table class="data-table">
        <thead><tr><th>Node</th><th>Build</th><th>Settings</th><th>Held</th><th>Lent</th></tr></thead>
        <tbody>${summary.nodes.map(node => renderNodeRow(node, masterFingerprint))}</tbody>
      </table>
    </div>
    <h4>Live leases</h4>
    ${leases.length > 0 ? html`<ul class="lending-leases">${leases.map(lease => renderLease(lease, now))}</ul>` : html`<p class="muted">No live leases.</p>`}
  `;
}

function renderNodeRow(node: LendingNodeSummary, masterFingerprint: string): SafeHTML {
  const held = node.held_requests ?? [];
  const inSync = masterFingerprint === "" || node.settings_fingerprint === masterFingerprint;
  return html`<tr>
    <td>${node.node_id}</td>
    <td>${node.build_version}</td>
    <td>${badge(inSync ? "in sync" : `differs (${node.settings_fingerprint})`, inSync ? "success" : "warning")}</td>
    <td>${held.filter(request => request.state === "held").length}</td>
    <td>${held.filter(request => request.state === "lent").length}</td>
  </tr>`;
}

function renderLease(lease: LendingLease, now: Date): SafeHTML {
  const remainingSeconds = Math.max(0, (new Date(lease.expires_at).getTime() - now.getTime()) / 1000);
  return html`<li>
    ${badge(lease.lane, laneAccent(lease.lane))}
    <span>${lease.owner_node_id}/${lease.owner_model_id} → ${lease.helper_node_id}/${lease.helper_model_id}</span>
    ${badge(`${lease.helper_slots} slot${lease.helper_slots === 1 ? "" : "s"}`, "neutral")}
    ${lease.probe ? badge("probe", "info") : ""}
    <span class="muted">expires in ${remainingSeconds.toFixed(0)}s</span>
  </li>`;
}
