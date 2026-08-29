import type { RoutingGroupCandidate, RoutingGroupMember, RoutingGroupsResponse } from "./types";

export interface CandidateNodeGroup {
  nodeId: string;
  candidates: RoutingGroupCandidate[];
}

export function candidateKey(member: RoutingGroupMember): string {
  return `${encodeURIComponent(member.node_id)}/${encodeURIComponent(member.image_id)}`;
}

export function memberFromCandidate(candidate: RoutingGroupCandidate): RoutingGroupMember {
  return {node_id: candidate.node_id, image_id: candidate.image_id};
}

/**
 * Candidates arrive unfiltered on purpose: the models worth grouping are usually
 * the ones a name or hash filter would have hidden. Splitting them here keeps that
 * decision visible, with the same weights first because that is the safe case.
 */
export function splitCandidatesByWeights(candidates: RoutingGroupCandidate[]): {
  sameWeights: RoutingGroupCandidate[];
  differentWeights: RoutingGroupCandidate[];
} {
  const sameWeights: RoutingGroupCandidate[] = [];
  const differentWeights: RoutingGroupCandidate[] = [];
  for (const candidate of candidates) {
    if (candidate.weights_match) {
      sameWeights.push(candidate);
    } else {
      differentWeights.push(candidate);
    }
  }
  return {sameWeights, differentWeights};
}

export function groupCandidatesByNode(candidates: RoutingGroupCandidate[]): CandidateNodeGroup[] {
  const byNode = new Map<string, RoutingGroupCandidate[]>();
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

export function selectedCandidateKeys(candidates: RoutingGroupCandidate[]): Set<string> {
  return new Set(candidates.filter(candidate => candidate.selected).map(candidate => candidateKey(candidate)));
}

/**
 * A group of one has nowhere to offload to, so the button reports peers rather
 * than members: what the operator cares about is how many other nodes can help.
 */
export function routingButtonLabel(peerCount: number): string {
  if (peerCount <= 0) {
    return "Routing";
  }
  if (peerCount === 1) {
    return "Routing · 1 peer";
  }
  return `Routing · ${peerCount} peers`;
}

export function peerCountForModel(response: RoutingGroupsResponse | null, member: RoutingGroupMember): number {
  if (!response) {
    return 0;
  }
  const key = candidateKey(member);
  for (const group of response.groups) {
    if (group.members.some(groupMember => candidateKey(groupMember) === key)) {
      return group.members.length - 1;
    }
  }
  return 0;
}

/**
 * The router cannot tell that two checkpoints differ, so the operator has to say
 * they understand before a mismatched member is saved. Only a newly added one asks
 * again: re-confirming a choice already in force would be noise.
 */
export function newlySelectedDifferentWeights(
  candidates: RoutingGroupCandidate[],
  selected: Set<string>
): RoutingGroupCandidate[] {
  return candidates.filter(
    candidate => !candidate.weights_match && selected.has(candidateKey(candidate)) && !candidate.selected
  );
}

export function membersFromSelection(candidates: RoutingGroupCandidate[], selected: Set<string>): RoutingGroupMember[] {
  return candidates
    .filter(candidate => selected.has(candidateKey(candidate)))
    .map(candidate => memberFromCandidate(candidate));
}
