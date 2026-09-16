import {
  candidateKey,
  groupCandidatesByNode,
  type LinkDraft,
  type LinkDraftField,
  type LinkDrafts,
  newlyLinkedDifferentWeights,
  routingLaneLabel
} from "./routing-links-data";
import type { RoutingCandidate, RoutingDirection, RoutingEndpoint, RoutingLane } from "./types";
import { escapeAttribute, escapeHTML } from "./utils";

export interface RoutingDialogView {
  lane: RoutingLane;
  anchor: RoutingEndpoint;
  candidates: RoutingCandidate[];
  drafts: LinkDrafts;
  acknowledged: boolean;
}

interface DirectionCopy {
  heading: string;
  note: (anchorID: string) => string;
  helperID: (anchorID: string, candidate: RoutingCandidate) => string;
}

const directionCopy: Record<RoutingDirection, DirectionCopy> = {
  lends_to: {
    heading: "Lends work to",
    note: anchorID => `Requests queued for ${anchorID} may run on these models while they are idle.`,
    helperID: (_anchorID, candidate) => candidate.model_id
  },
  borrows_from: {
    heading: "Borrows work from",
    note: anchorID => `${anchorID} may run requests queued for these models while it is idle.`,
    helperID: anchorID => anchorID
  }
};

export function routingDialogMarkup(view: RoutingDialogView): string {
  const pendingMismatch = newlyLinkedDifferentWeights(view.candidates, view.drafts);
  const saveBlocked = pendingMismatch.length > 0 && !view.acknowledged;
  return `
    <h2>${escapeHTML(routingLaneLabel(view.lane))} routing · ${escapeHTML(view.anchor.model_id)}</h2>
    <p class="dialog-note">A request for ${escapeHTML(view.anchor.model_id)} always goes to its own node first. Only requests still waiting in its queue are lent, and borrowed work is handed back as soon as the helper has work of its own.</p>
    ${directionSectionMarkup(view, "lends_to")}
    ${directionSectionMarkup(view, "borrows_from")}
    ${pendingMismatch.length > 0 ? acknowledgementMarkup(pendingMismatch, view.acknowledged) : ""}
    <div class="dialog-actions">
      <button type="button" data-routing-action="cancel">Cancel</button>
      <button type="button" data-routing-action="save"${saveBlocked ? " disabled" : ""}>Save links</button>
    </div>
  `;
}

function directionSectionMarkup(view: RoutingDialogView, direction: RoutingDirection): string {
  const copy = directionCopy[direction];
  const body = view.candidates.length === 0
    ? `<p class="dialog-note">No models on other nodes are available to link with.</p>`
    : groupCandidatesByNode(view.candidates)
      .map(group => {
        const rows = group.candidates.map(candidate => candidateRowMarkup(view, direction, candidate)).join("");
        return `<div class="routing-node"><h3>${escapeHTML(group.nodeId)}</h3>${rows}</div>`;
      })
      .join("");
  return `
    <section class="routing-direction">
      <h3 class="routing-direction-heading">${escapeHTML(copy.heading)}</h3>
      <p class="dialog-note">${escapeHTML(copy.note(view.anchor.model_id))}</p>
      ${body}
    </section>
  `;
}

function candidateRowMarkup(view: RoutingDialogView, direction: RoutingDirection, candidate: RoutingCandidate): string {
  const key = candidateKey(candidate);
  const draft = view.drafts[direction].get(key) ?? {selected: false, loadIfUnloaded: true, restoreAfterBorrow: false};
  const helperID = directionCopy[direction].helperID(view.anchor.model_id, candidate);
  const flagsEnabled = candidate.eligible && draft.selected;
  return `
    <div class="routing-candidate-row">
      <label class="routing-candidate">
        ${checkboxMarkup(direction, key, "selected", draft, candidate.eligible)}
        <span class="routing-candidate-name">${escapeHTML(candidate.model_id)}<span>${escapeHTML(candidateDetails(candidate))}</span></span>
        ${weightsBadgeMarkup(candidate)}
      </label>
      ${candidate.eligible ? "" : `<p class="routing-candidate-ineligible">${escapeHTML(candidate.ineligible_reason ?? "Not eligible")}</p>`}
      <label class="routing-candidate-flag">
        ${checkboxMarkup(direction, key, "loadIfUnloaded", draft, flagsEnabled)}
        <span>Load ${escapeHTML(helperID)} for lent work when it is not loaded</span>
      </label>
      <label class="routing-candidate-flag">
        ${checkboxMarkup(direction, key, "restoreAfterBorrow", draft, flagsEnabled)}
        <span>Reload what ${escapeHTML(helperID)} displaced once borrowed work goes quiet</span>
      </label>
    </div>
  `;
}

function checkboxMarkup(direction: RoutingDirection, key: string, field: LinkDraftField, draft: LinkDraft, enabled: boolean): string {
  return `<input type="checkbox" data-routing-direction="${direction}" data-routing-key="${escapeAttribute(key)}" data-routing-field="${field}"${draft[field] ? " checked" : ""}${enabled ? "" : " disabled"}>`;
}

function candidateDetails(candidate: RoutingCandidate): string {
  return candidate.context_size ? `${candidate.filename} · ${candidate.context_size} ctx` : candidate.filename;
}

function weightsBadgeMarkup(candidate: RoutingCandidate): string {
  return candidate.weights_match
    ? `<span class="routing-badge routing-badge-same">same weights</span>`
    : `<span class="routing-badge routing-badge-different">different weights</span>`;
}

function acknowledgementMarkup(pending: RoutingCandidate[], acknowledged: boolean): string {
  const names = pending.map(candidate => candidate.model_id).join(", ");
  return `
    <label class="routing-acknowledge">
      <input type="checkbox" data-routing-acknowledge${acknowledged ? " checked" : ""}>
      <span>I understand ${escapeHTML(names)} ${pending.length === 1 ? "has" : "have"} different weights and will return different output for lent work.</span>
    </label>
  `;
}
