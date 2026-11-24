import { api, apiBlob } from "./client";
import type {
  SecurityAuthorityCreateRequest,
  SecurityCertificate,
  SecurityCertificateImportRequest,
  SecurityCertificatesResponse,
  SecurityIdentityGenerateRequest,
  SecurityIssueRequest,
  SecurityIssuedIdentity,
  SecurityStatus,
  SecurityTokenCreateRequest,
  SecurityTokenCreatedResponse,
  SecurityTokenSecretResponse,
  SecurityTokensResponse
} from "../types";

const securityBase = "/api/security";

function post<T>(path: string, body: unknown): Promise<T> {
  return api<T>(`${securityBase}${path}`, {method: "POST", body: JSON.stringify(body)});
}

function resourcePath(collection: string, id: string, action = ""): string {
  const resource = `/${collection}/${encodeURIComponent(id)}`;
  return action ? `${resource}/${action}` : resource;
}

export function getSecurityStatus(): Promise<SecurityStatus> {
  return api<SecurityStatus>(`${securityBase}/status`);
}

export function switchSecurityProfileToSecure(): Promise<SecurityStatus> {
  return post<SecurityStatus>("/profile", {});
}

export function getSecurityTokens(): Promise<SecurityTokensResponse> {
  return api<SecurityTokensResponse>(`${securityBase}/tokens`);
}

export function createSecurityToken(request: SecurityTokenCreateRequest): Promise<SecurityTokenCreatedResponse> {
  return post<SecurityTokenCreatedResponse>("/tokens", request);
}

export function deleteSecurityToken(id: string): Promise<null> {
  return api<null>(`${securityBase}${resourcePath("tokens", id)}`, {method: "DELETE"});
}

export function revealClusterTokenSecret(id: string): Promise<SecurityTokenSecretResponse> {
  return api<SecurityTokenSecretResponse>(`${securityBase}${resourcePath("tokens", id, "secret")}`);
}

export function getSecurityCertificates(): Promise<SecurityCertificatesResponse> {
  return api<SecurityCertificatesResponse>(`${securityBase}/certificates`);
}

export function importSecurityCertificate(request: SecurityCertificateImportRequest): Promise<SecurityCertificate> {
  return post<SecurityCertificate>("/certificates", request);
}

export function generateSecurityIdentity(request: SecurityIdentityGenerateRequest): Promise<SecurityCertificate> {
  return post<SecurityCertificate>("/certificates/generate", request);
}

export function activateSecurityCertificate(id: string): Promise<null> {
  return post<null>(resourcePath("certificates", id, "activate"), {});
}

export function deleteSecurityCertificate(id: string, force: boolean): Promise<null> {
  const query = force ? "?force=true" : "";
  return api<null>(`${securityBase}${resourcePath("certificates", id)}${query}`, {method: "DELETE"});
}

export function createSecurityAuthority(request: SecurityAuthorityCreateRequest): Promise<SecurityCertificate> {
  return post<SecurityCertificate>("/authorities", request);
}

export function issueSecurityIdentity(authorityID: string, request: SecurityIssueRequest): Promise<SecurityIssuedIdentity> {
  return post<SecurityIssuedIdentity>(resourcePath("authorities", authorityID, "issue"), request);
}

export function installIssuedSecurityIdentity(authorityID: string, request: SecurityIssueRequest): Promise<SecurityCertificate> {
  return post<SecurityCertificate>(resourcePath("authorities", authorityID, "issue"), {...request, install_locally: true});
}

export function downloadSecurityCertificatePEM(id: string): Promise<Blob> {
  return apiBlob(`${securityBase}${resourcePath("certificates", id, "pem")}`);
}
