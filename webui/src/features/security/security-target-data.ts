import type { SecurityClusterTokenPushResponse, SecurityTarget } from "../../types";

export const localSecurityTarget: SecurityTarget = {scope: "local"};

export function securityTargetFor(nodeID: string): SecurityTarget {
  return nodeID === "" ? localSecurityTarget : {scope: "node", nodeID};
}

export function securityTargetSubject(target: SecurityTarget): string {
  return target.scope === "local" ? "this router" : `node ${target.nodeID}`;
}

export function clusterTokenPushMessage(nodeID: string, response: SecurityClusterTokenPushResponse): string {
  return response.created
    ? `Sent the cluster token to ${nodeID} as ${response.token.name}`
    : `${nodeID} already holds the cluster token as ${response.token.name}`;
}
