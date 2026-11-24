import { api, apiBlob } from "./client";
import { securityResourcePath, securityTargetPath } from "./security-paths";
import type {
  SecurityCertificate,
  SecurityCertificateRequestCreate,
  SecurityIdentityGenerateRequest,
  SecurityPendingKey,
  SecuritySignedCertificateInstall,
  SecuritySummary,
  SecurityTarget,
  SecurityTokenCreateRequest,
  SecurityTokenCreatedResponse
} from "../types";

function post<T>(target: SecurityTarget, path: string, body: unknown): Promise<T> {
  return api<T>(`${securityTargetPath(target)}${path}`, {method: "POST", body: JSON.stringify(body)});
}

function remove(target: SecurityTarget, path: string): Promise<null> {
  return api<null>(`${securityTargetPath(target)}${path}`, {method: "DELETE"});
}

export function getSecuritySummary(target: SecurityTarget): Promise<SecuritySummary> {
  return api<SecuritySummary>(`${securityTargetPath(target)}/summary`);
}

export function createSecurityToken(target: SecurityTarget, request: SecurityTokenCreateRequest): Promise<SecurityTokenCreatedResponse> {
  return post<SecurityTokenCreatedResponse>(target, "/tokens", request);
}

export function deleteSecurityToken(target: SecurityTarget, id: string): Promise<null> {
  return remove(target, securityResourcePath("tokens", id));
}

export function generateSecurityIdentity(target: SecurityTarget, request: SecurityIdentityGenerateRequest): Promise<SecurityCertificate> {
  return post<SecurityCertificate>(target, "/certificates/generate", request);
}

export function activateSecurityCertificate(target: SecurityTarget, id: string): Promise<unknown> {
  return post<unknown>(target, securityResourcePath("certificates", id, "activate"), {});
}

export function deleteSecurityCertificate(target: SecurityTarget, id: string, force: boolean): Promise<null> {
  const query = force ? "?force=true" : "";
  return remove(target, `${securityResourcePath("certificates", id)}${query}`);
}

export function downloadSecurityCertificatePEM(target: SecurityTarget, id: string): Promise<Blob> {
  return apiBlob(`${securityTargetPath(target)}${securityResourcePath("certificates", id, "pem")}`);
}

export function createSecurityCertificateRequest(target: SecurityTarget, request: SecurityCertificateRequestCreate): Promise<SecurityPendingKey> {
  return post<SecurityPendingKey>(target, "/csr", request);
}

export function deleteSecurityCertificateRequest(target: SecurityTarget, id: string): Promise<null> {
  return remove(target, securityResourcePath("csr", id));
}

export function installSignedSecurityCertificate(target: SecurityTarget, request: SecuritySignedCertificateInstall): Promise<SecurityCertificate> {
  return post<SecurityCertificate>(target, "/install", request);
}
