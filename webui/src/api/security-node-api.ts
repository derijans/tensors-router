import { api } from "./client";
import { securityTargetPath } from "./security-paths";
import type { SecurityCertificate, SecurityClusterTokenPushResponse, SecurityNodeIssueRequest } from "../types";

function postToNode<T>(nodeID: string, path: string, body: unknown): Promise<T> {
  return api<T>(`${securityTargetPath({scope: "node", nodeID})}${path}`, {method: "POST", body: JSON.stringify(body)});
}

export function trustMasterAuthority(nodeID: string, certificateID: string): Promise<SecurityCertificate> {
  return postToNode<SecurityCertificate>(nodeID, "/trust", {certificate_id: certificateID});
}

export function pushClusterToken(nodeID: string): Promise<SecurityClusterTokenPushResponse> {
  return postToNode<SecurityClusterTokenPushResponse>(nodeID, "/cluster-token", {});
}

export function issueFromMasterAuthority(nodeID: string, request: SecurityNodeIssueRequest): Promise<SecurityCertificate> {
  return postToNode<SecurityCertificate>(nodeID, "/issue", request);
}
