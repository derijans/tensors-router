import { deleteRoutingLinks, fetchRoutingLinks, saveRoutingLinks } from "./api";
import { elements } from "./elements";
import {
  draftsFromCandidates,
  type LinkDraftField,
  linksRequest,
  requestHasNoLinks,
  updateLinkDraft
} from "./routing-links-data";
import { type RoutingDialogView, routingDialogMarkup } from "./routing-links-markup";
import type { RoutingDirection, RoutingEndpoint, RoutingLane } from "./types";

let session: RoutingDialogView | null = null;

const linkDraftFields: LinkDraftField[] = ["selected", "loadIfUnloaded", "restoreAfterBorrow"];

function isRoutingDirection(value: string | undefined): value is RoutingDirection {
  return value === "lends_to" || value === "borrows_from";
}

function isLinkDraftField(value: string | undefined): value is LinkDraftField {
  return linkDraftFields.some(field => field === value);
}

export function registerRoutingLinksDialog(refreshInventory: () => Promise<void>): void {
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
    const {routingDirection, routingKey, routingField} = target.dataset;
    if (!isRoutingDirection(routingDirection) || routingKey === undefined || !isLinkDraftField(routingField)) {
      return;
    }
    updateLinkDraft(session.drafts, routingDirection, routingKey, routingField, target.checked);
    if (routingField === "selected") {
      session.acknowledged = false;
    }
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
      void submitRoutingLinks(refreshInventory);
    }
  });
}

export async function openRoutingLinksDialog(lane: RoutingLane, anchor: RoutingEndpoint): Promise<void> {
  const response = await fetchRoutingLinks(lane, anchor);
  const candidates = response.candidates ?? [];
  session = {
    lane,
    anchor: response.anchor ?? anchor,
    candidates,
    drafts: draftsFromCandidates(candidates),
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

async function submitRoutingLinks(refreshInventory: () => Promise<void>): Promise<void> {
  if (!session) {
    return;
  }
  const request = linksRequest(session.anchor, session.candidates, session.drafts);
  setRoutingStatus("Saving routing links…", false);
  try {
    if (requestHasNoLinks(request)) {
      await deleteRoutingLinks(session.lane, session.anchor);
    } else {
      await saveRoutingLinks(session.lane, request);
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
  elements.routingDialogBody.innerHTML = routingDialogMarkup(session);
  setRoutingStatus("", false);
}
