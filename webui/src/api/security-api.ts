import { api } from "./client";
import { securityResourcePath, securityTargetPath } from "./security-paths";
import type {
  SecurityAuthorityCreateRequest,
  SecurityCertificate,
  SecurityCertificateImportRequest,
  SecurityIssueRequest,
  SecurityIssuedIdentity,
  SecurityStatus,
  SecurityTokenSecretResponse
} from "../types";

const localBase = securityTargetPath({scope: "local"});

function post<T>(path: string, body: unknown): Promise<T> {
  return api<T>(`${localBase}${path}`, {method: "POST", body: JSON.stringify(body)});
}

export function switchSecurityProfileToSecure(): Promise<SecurityStatus> {
  return post<SecurityStatus>("/profile", {});
}

export function revealClusterTokenSecret(id: string): Promise<SecurityTokenSecretResponse> {
  return api<SecurityTokenSecretResponse>(`${localBase}${securityResourcePath("tokens", id, "secret")}`);
}

export function importSecurityCertificate(request: SecurityCertificateImportRequest): Promise<SecurityCertificate> {
  return post<SecurityCertificate>("/certificates", request);
}

export function createSecurityAuthority(request: SecurityAuthorityCreateRequest): Promise<SecurityCertificate> {
  return post<SecurityCertificate>("/authorities", request);
}

export function issueSecurityIdentity(authorityID: string, request: SecurityIssueRequest): Promise<SecurityIssuedIdentity> {
  return post<SecurityIssuedIdentity>(securityResourcePath("authorities", authorityID, "issue"), request);
}

export function installIssuedSecurityIdentity(authorityID: string, request: SecurityIssueRequest): Promise<SecurityCertificate> {
  return post<SecurityCertificate>(securityResourcePath("authorities", authorityID, "issue"), {...request, install_locally: true});
}
