import type { NodeRequest, NodeRequestState } from "./types";

export interface ModelRequestGroup {
  lane: string;
  modelID: string;
  counts: Map<NodeRequestState, number>;
  oldestAgeMS: number;
  helpers: string[];
}

const runningStates: readonly NodeRequestState[] = ["running", "borrowed"];

export function groupRequestsByModel(requests: readonly NodeRequest[]): ModelRequestGroup[] {
  const groups = new Map<string, ModelRequestGroup>();
  for (const request of requests) {
    const key = `${request.lane}\u0000${request.model_id}`;
    const group = groups.get(key) ?? emptyGroup(request);
    group.counts.set(request.state, (group.counts.get(request.state) ?? 0) + 1);
    group.oldestAgeMS = Math.max(group.oldestAgeMS, request.age_ms);
    const helper = lentHelper(request);
    if (helper !== "" && !group.helpers.includes(helper)) {
      group.helpers.push(helper);
    }
    groups.set(key, group);
  }
  return [...groups.values()].sort((left, right) => right.oldestAgeMS - left.oldestAgeMS);
}

export function requestsOldestFirst(requests: readonly NodeRequest[]): NodeRequest[] {
  return [...requests].sort((left, right) => right.age_ms - left.age_ms);
}

export function runningRequestCount(requests: readonly NodeRequest[], modelID: string): number {
  return requests.filter(request => request.model_id === modelID && runningStates.includes(request.state)).length;
}

export function requestsForUnloadedModels(requests: readonly NodeRequest[], loadedModelIDs: ReadonlySet<string>): ModelRequestGroup[] {
  return groupRequestsByModel(requests.filter(request => !loadedModelIDs.has(request.model_id)));
}

function emptyGroup(request: NodeRequest): ModelRequestGroup {
  return {lane: request.lane, modelID: request.model_id, counts: new Map<NodeRequestState, number>(), oldestAgeMS: 0, helpers: []};
}

function lentHelper(request: NodeRequest): string {
  if (request.state !== "lent") {
    return "";
  }
  return `${request.helper_node_id || "?"}/${request.helper_model_id || "?"}`;
}
