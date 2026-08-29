import { deleteRoutingGroup, fetchRoutingGroups, saveRoutingGroup } from "./api";
import { elements } from "./elements";
import {
  candidateKey,
  groupCandidatesByNode,
  membersFromSelection,
  newlySelectedDifferentWeights,
  selectedCandidateKeys
} from "./routing-groups-data";
import type { RoutingGroupCandidate, RoutingGroupMember } from "./types";
import { escapeAttribute, escapeHTML } from "./utils";

interface DialogSession {
  anchor: RoutingGroupMember;
  candidates: RoutingGroupCandidate[];
  selected: Set<string>;
  acknowledged: boolean;
}

let session: DialogSession | null = null;

export function registerRoutingGroupDialog(refreshInventory: () => Promise<void>): void {
  elements.routingDialog.addEventListener("cancel", event => {
    event.preventDefault();
    closeRoutingDialog();
  });
  elements.routingDialog.addEventListener("change", event => {
    const target = event.target;
    if (!(target instanceof HTMLInputElement) || !session) {
      return;
    }
    if (target.dataset.routingAcknowledge !== undefined) {
      session.acknowledged = target.checked;
      renderRoutingDialog();
      return;
    }
    const key = target.dataset.routingCandidate;
    if (key === undefined) {
      return;
    }
    if (target.checked) {
      session.selected.add(key);
    } else {
      session.selected.delete(key);
    }
    session.acknowledged = false;
    renderRoutingDialog();
  });
  elements.routingDialog.addEventListener("click", event => {
    const target = event.target;
    if (!(target instanceof HTMLElement) || !session) {
      return;
    }
    if (target.dataset.routingAction === "cancel") {
      closeRoutingDialog();
      return;
    }
    if (target.dataset.routingAction === "save") {
      void submitRoutingGroup(refreshInventory);
    }
  });
}

export async function openRoutingGroupDialog(anchor: RoutingGroupMember): Promise<void> {
  const response = await fetchRoutingGroups(anchor);
  const candidates = response.candidates ?? [];
  session = {
    anchor,
    candidates,
    selected: selectedCandidateKeys(candidates),
    acknowledged: false
  };
  renderRoutingDialog();
  elements.routingDialog.showModal();
}

function closeRoutingDialog(): void {
  session = null;
  if (elements.routingDialog.open) {
    elements.routingDialog.close();
  }
}

async function submitRoutingGroup(refreshInventory: () => Promise<void>): Promise<void> {
  if (!session) {
    return;
  }
  const members = membersFromSelection(session.candidates, session.selected);
  setRoutingStatus("Saving routing group…", false);
  try {
    if (members.length === 0) {
      await deleteRoutingGroup(session.anchor);
    } else {
      await saveRoutingGroup({anchor: session.anchor, members});
    }
    closeRoutingDialog();
    await refreshInventory();
  } catch (error) {
    setRoutingStatus(error instanceof Error ? error.message : String(error), true);
  }
}

function setRoutingStatus(message: string, error: boolean): void {
  elements.routingDialogStatus.textContent = message;
  elements.routingDialogStatus.classList.toggle("error-text", error);
}

function renderRoutingDialog(): void {
  if (!session) {
    return;
  }
  const pendingMismatch = newlySelectedDifferentWeights(session.candidates, session.selected);
  const saveBlocked = pendingMismatch.length > 0 && !session.acknowledged;
  elements.routingDialogBody.innerHTML = `
    <h2>Routing group · ${escapeHTML(session.anchor.image_id)}</h2>
    <p class="dialog-note">Requests for this model may be served by any node below while that node is idle. Borrowed work is handed back as soon as the node has its own.</p>
    ${candidateMarkup(session)}
    ${pendingMismatch.length > 0 ? acknowledgementMarkup(pendingMismatch, session.acknowledged) : ""}
    <div class="dialog-actions">
      <button type="button" data-routing-action="cancel">Cancel</button>
      <button type="button" data-routing-action="save"${saveBlocked ? " disabled" : ""}>Save group</button>
    </div>
  `;
  setRoutingStatus("", false);
}

function candidateMarkup(current: DialogSession): string {
  if (current.candidates.length === 0) {
    return `<p class="dialog-note">No image models on other nodes are available to group with.</p>`;
  }
  return groupCandidatesByNode(current.candidates)
    .map(group => {
      const rows = group.candidates.map(candidate => candidateRow(candidate, current.selected)).join("");
      return `<div class="routing-node"><h3>${escapeHTML(group.nodeId)}</h3>${rows}</div>`;
    })
    .join("");
}

function candidateRow(candidate: RoutingGroupCandidate, selected: Set<string>): string {
  const key = candidateKey(candidate);
  const badge = candidate.weights_match
    ? `<span class="routing-badge routing-badge-same">same weights</span>`
    : `<span class="routing-badge routing-badge-different">different weights</span>`;
  return `
    <label class="routing-candidate">
      <input type="checkbox" data-routing-candidate="${escapeAttribute(key)}"${selected.has(key) ? " checked" : ""}>
      <span class="routing-candidate-name">${escapeHTML(candidate.image_id)}<span>${escapeHTML(candidate.filename)}</span></span>
      ${badge}
    </label>
  `;
}

function acknowledgementMarkup(pending: RoutingGroupCandidate[], acknowledged: boolean): string {
  const names = pending.map(candidate => candidate.image_id).join(", ");
  return `
    <label class="routing-acknowledge">
      <input type="checkbox" data-routing-acknowledge${acknowledged ? " checked" : ""}>
      <span>I understand ${escapeHTML(names)} ${pending.length === 1 ? "has" : "have"} different weights and will return different images for this model ID.</span>
    </label>
  `;
}
