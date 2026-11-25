import { api } from "./client";
import { securityTargetPath } from "./security-paths";
import type { SecurityCertificate, SecurityNodeIssueRequest, SecurityProfileStatus, SecurityTarget } from "../types";

export function issueFromAuthority(target: SecurityTarget, request: SecurityNodeIssueRequest): Promise<SecurityCertificate> {
  return api<SecurityCertificate>(`${securityTargetPath(target)}/issue`, {method: "POST", body: JSON.stringify(request)});
}

export function switchProfileToSecure(target: SecurityTarget): Promise<SecurityProfileStatus> {
  return api<SecurityProfileStatus>(`${securityTargetPath(target)}/profile`, {method: "POST", body: "{}"});
}
