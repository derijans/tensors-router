import type {
  RoutingCandidate,
  RoutingDirection,
  RoutingEndpoint,
  RoutingLane,
  RoutingLinkChoice,
  RoutingLinksRequest,
  RoutingLinksResponse
} from "./types";

export const routingDirections: RoutingDirection[] = ["lends_to", "borrows_from"];

export interface LinkDraft {
  selected: boolean;
  loadIfUnloaded: boolean;
  restoreAfterBorrow: boolean;
}

export type LinkDrafts = Record<RoutingDirection, Map<string, LinkDraft>>;

export type LinkDraftField = keyof LinkDraft;

export interface CandidateNodeGroup {
  nodeId: string;
  candidates: RoutingCandidate[];
}

export interface LinkCounts {
  lendsTo: number;
  borrowsFrom: number;
}

export function candidateKey(endpoint: RoutingEndpoint): string {
  return `${encodeURIComponent(endpoint.node_id)}/${encodeURIComponent(endpoint.model_id)}`;
}

export function routingLaneLabel(lane: RoutingLane): string {
  return lane === "image" ? "Image" : "LLM";
}

export function groupCandidatesByNode(candidates: RoutingCandidate[]): CandidateNodeGroup[] {
  const byNode = new Map<string, RoutingCandidate[]>();
  for (const candidate of candidates) {
    const existing = byNode.get(candidate.node_id);
    if (existing) {
      existing.push(candidate);
    } else {
      byNode.set(candidate.node_id, [candidate]);
    }
  }
  return [...byNode.entries()]
    .map(([nodeId, nodeCandidates]) => ({nodeId, candidates: nodeCandidates}))
    .sort((left, right) => left.nodeId.localeCompare(right.nodeId));
}

export function draftsFromCandidates(candidates: RoutingCandidate[]): LinkDrafts {
  const drafts: LinkDrafts = {lends_to: new Map(), borrows_from: new Map()};
  for (const candidate of candidates) {
    for (const direction of routingDirections) {
      const saved = candidate[direction];
      drafts[direction].set(candidateKey(candidate), {
        selected: saved.selected,
        loadIfUnloaded: saved.selected ? saved.load_if_unloaded : true,
        restoreAfterBorrow: saved.restore_after_borrow
      });
    }
  }
  return drafts;
}

export function updateLinkDraft(drafts: LinkDrafts, direction: RoutingDirection, key: string, field: LinkDraftField, value: boolean): void {
  const current = drafts[direction].get(key);
  if (!current) {
    return;
  }
  drafts[direction].set(key, {...current, [field]: value});
}

function choicesFor(candidates: RoutingCandidate[], drafts: Map<string, LinkDraft>): RoutingLinkChoice[] {
  return candidates.flatMap(candidate => {
    const draft = drafts.get(candidateKey(candidate));
    if (!candidate.eligible || !draft?.selected) {
      return [];
    }
    return [{
      node_id: candidate.node_id,
      model_id: candidate.model_id,
      load_if_unloaded: draft.loadIfUnloaded,
      restore_after_borrow: draft.restoreAfterBorrow
    }];
  });
}

export function linksRequest(anchor: RoutingEndpoint, candidates: RoutingCandidate[], drafts: LinkDrafts): RoutingLinksRequest {
  return {
    anchor,
    lends_to: choicesFor(candidates, drafts.lends_to),
    borrows_from: choicesFor(candidates, drafts.borrows_from)
  };
}

export function requestHasNoLinks(request: RoutingLinksRequest): boolean {
  return request.lends_to.length === 0 && request.borrows_from.length === 0;
}

/**
 * The router cannot tell that two checkpoints differ, so the operator has to say
 * they understand before a mismatched peer is linked. Only a peer with no saved link
 * in either direction asks: re-confirming a choice already in force would be noise.
 */
export function newlyLinkedDifferentWeights(candidates: RoutingCandidate[], drafts: LinkDrafts): RoutingCandidate[] {
  return candidates.filter(candidate => {
    if (candidate.weights_match || candidate.lends_to.selected || candidate.borrows_from.selected) {
      return false;
    }
    const key = candidateKey(candidate);
    return routingDirections.some(direction => drafts[direction].get(key)?.selected === true);
  });
}

export function linkCountsForModel(response: RoutingLinksResponse | null, endpoint: RoutingEndpoint): LinkCounts {
  const counts: LinkCounts = {lendsTo: 0, borrowsFrom: 0};
  const key = candidateKey(endpoint);
  for (const link of response?.links ?? []) {
    if (candidateKey(link.owner) === key) {
      counts.lendsTo++;
    }
    if (candidateKey(link.helper) === key) {
      counts.borrowsFrom++;
    }
  }
  return counts;
}

export function routingButtonLabel(lane: RoutingLane, counts: LinkCounts): string {
  const label = `${routingLaneLabel(lane)} routing`;
  if (counts.lendsTo === 0 && counts.borrowsFrom === 0) {
    return label;
  }
  return `${label} · lends ${counts.lendsTo} · borrows ${counts.borrowsFrom}`;
}
