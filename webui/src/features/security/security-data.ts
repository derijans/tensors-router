import type { SecurityCertificate, SecurityCertificateKind, SecurityStatus, SecurityToken, SecurityTokenRole, Tone } from "../../types";

const dayMilliseconds = 24 * 60 * 60 * 1000;
const expiryWarningDays = 30;

export const tokenRoles: readonly SecurityTokenRole[] = ["inference", "admin", "metrics", "cluster"];

export const importableKinds: readonly SecurityCertificateKind[] = ["identity", "authority", "trusted_authority", "pinned_peer"];

const roleLabels: Record<SecurityTokenRole, string> = {
  inference: "Inference",
  admin: "Admin",
  metrics: "Metrics",
  cluster: "Cluster"
};

const kindLabels: Record<SecurityCertificateKind, string> = {
  identity: "Router identity",
  authority: "Local CA",
  trusted_authority: "Trusted CA",
  pinned_peer: "Pinned peer"
};

const kindOrder: Record<SecurityCertificateKind, number> = {
  identity: 0,
  authority: 1,
  trusted_authority: 2,
  pinned_peer: 3
};

export interface ExpiryStatus {
  label: string;
  tone: Tone;
}

export interface ListenerNotice {
  message: string;
  tone: Tone;
}

export function isTokenRole(value: string): value is SecurityTokenRole {
  return (tokenRoles as readonly string[]).includes(value);
}

export function isCertificateKind(value: string): value is SecurityCertificateKind {
  return (importableKinds as readonly string[]).includes(value);
}

export function roleLabel(role: SecurityTokenRole): string {
  return roleLabels[role];
}

export function kindLabel(kind: SecurityCertificateKind): string {
  return kindLabels[kind];
}

export function sortedTokens(tokens: readonly SecurityToken[]): SecurityToken[] {
  return [...tokens].sort((left, right) =>
    tokenRoles.indexOf(left.role) - tokenRoles.indexOf(right.role) || left.name.localeCompare(right.name));
}

export function sortedCertificates(certificates: readonly SecurityCertificate[]): SecurityCertificate[] {
  return [...certificates].sort((left, right) =>
    Number(right.active) - Number(left.active) || kindOrder[left.kind] - kindOrder[right.kind] || left.name.localeCompare(right.name));
}

export function authorities(certificates: readonly SecurityCertificate[]): SecurityCertificate[] {
  return certificates.filter(certificate => certificate.kind === "authority");
}

export function certificateHosts(certificate: SecurityCertificate): string[] {
  return [...certificate.dns_names, ...certificate.ip_addresses];
}

export function shortFingerprint(fingerprint: string): string {
  return fingerprint.slice(0, 16).replace(/(.{4})(?=.)/g, "$1:");
}

export function expiryStatus(notAfter: string, now: Date): ExpiryStatus {
  const remainingDays = Math.floor((new Date(notAfter).getTime() - now.getTime()) / dayMilliseconds);
  if (remainingDays < 0) {
    return {label: "Expired", tone: "danger"};
  }
  if (remainingDays <= expiryWarningDays) {
    return {label: `Expires in ${remainingDays} d`, tone: "warning"};
  }
  return {label: new Date(notAfter).toLocaleDateString(), tone: "neutral"};
}

export function canRevealSecret(token: SecurityToken): boolean {
  return token.role === "cluster";
}

export function canActivate(certificate: SecurityCertificate): boolean {
  return certificate.kind === "identity" && !certificate.active;
}

export function canIssue(certificate: SecurityCertificate): boolean {
  return certificate.kind === "authority";
}

export function parseHosts(value: string): string[] {
  return value.split(/[\s,]+/).map(host => host.trim()).filter(host => host !== "");
}

export function listenerNotice(status: SecurityStatus | null): ListenerNotice {
  if (!status) {
    return {message: "", tone: "neutral"};
  }
  if (status.restart_required && status.identity_active) {
    return {message: "An identity is active but the router still serves plain HTTP. Restart the router to switch to HTTPS.", tone: "warning"};
  }
  if (status.restart_required) {
    return {message: "No identity is active but the listener still uses HTTPS with the previous certificate. Restart the router to apply.", tone: "warning"};
  }
  if (status.listener_tls) {
    return {message: "HTTPS with mutual TLS for cluster peers is active.", tone: "success"};
  }
  return {message: "Plain HTTP. Activate a router identity so traffic between routers is encrypted.", tone: "info"};
}

export function pemFilename(name: string, suffix: string): string {
  const safeName = name.trim().replace(/[^A-Za-z0-9._-]+/g, "_") || "certificate";
  return `${safeName}${suffix}`;
}
