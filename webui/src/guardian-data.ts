import type { GuardianAssignment, GuardianEndpoint, GuardianScope, InventoryResponse } from "./types";

export interface GuardianTarget {
  scope: GuardianScope;
  nodeID: string;
  modelID: string;
  label: string;
}

export interface GuardianRow {
  target: GuardianTarget;
  assigned: GuardianEndpoint | null;
  effective: GuardianEndpoint | null;
}

const clusterTarget: GuardianTarget = {scope: "cluster", nodeID: "", modelID: "", label: "Whole cluster"};

export function guardianTargets(inventory: InventoryResponse | null): GuardianTarget[] {
  const nodes = (inventory?.nodes ?? []).map(node => node.node_id).filter(nodeID => nodeID !== "").sort();
  const models = (inventory?.models ?? [])
    .filter(model => model.has_llm && !model.has_multimodal && model.node_id !== "")
    .sort((left, right) => `${left.node_id}\u0000${left.local_id}`.localeCompare(`${right.node_id}\u0000${right.local_id}`));
  return [
    clusterTarget,
    ...nodes.map(nodeID => ({scope: "node" as const, nodeID, modelID: "", label: `Node ${nodeID}`})),
    ...models.map(model => ({scope: "model" as const, nodeID: model.node_id, modelID: model.local_id, label: `${model.node_id} / ${model.local_id}`}))
  ];
}

export function guardianRows(targets: readonly GuardianTarget[], assignments: readonly GuardianAssignment[]): GuardianRow[] {
  const find = (scope: GuardianScope, nodeID: string, modelID: string): GuardianEndpoint | null =>
    assignments.find(assignment => assignment.scope === scope && (assignment.node_id ?? "") === nodeID && (assignment.model_id ?? "") === modelID)?.guardian ?? null;
  return targets.map(target => {
    const assigned = find(target.scope, target.nodeID, target.modelID);
    const effective = assigned
      ?? (target.scope === "model" ? find("node", target.nodeID, "") : null)
      ?? (target.scope !== "cluster" ? find("cluster", "", "") : null);
    return {target, assigned, effective};
  });
}

export function guardianAssignment(target: GuardianTarget, guardian: GuardianEndpoint): GuardianAssignment {
  return {scope: target.scope, node_id: target.nodeID, model_id: target.modelID, guardian};
}

export function endpointValue(endpoint: GuardianEndpoint): string {
  return JSON.stringify([endpoint.node_id, endpoint.model_id]);
}

export function parseEndpointValue(value: string): GuardianEndpoint | null {
  try {
    const parsed: unknown = JSON.parse(value);
    if (Array.isArray(parsed) && parsed.length === 2 && typeof parsed[0] === "string" && typeof parsed[1] === "string") {
      return {node_id: parsed[0], model_id: parsed[1]};
    }
  } catch {
    return null;
  }
  return null;
}

export function endpointLabel(endpoint: GuardianEndpoint): string {
  return `${endpoint.node_id} / ${endpoint.model_id}`;
}
