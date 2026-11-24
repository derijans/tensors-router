import type {
  SecurityCertificate,
  SecurityCertificateKind,
  SecurityListener,
  SecurityRequestKind,
  SecurityStatus,
  SecurityTarget,
  SecurityToken,
  SecurityTokenRole,
  Tone
} from "../../types";

const dayMilliseconds = 24 * 60 * 60 * 1000;
export const expiryWarningDays = 30;

export const tokenRoles: readonly SecurityTokenRole[] = ["inference", "admin", "metrics", "cluster"];

export const importableKinds: readonly SecurityCertificateKind[] = ["api_certificate", "identity", "authority", "trusted_authority", "pinned_peer"];

export const requestKinds: readonly SecurityRequestKind[] = ["identity", "api_certificate"];

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
  pinned_peer: "Pinned peer",
  api_certificate: "API certificate"
};

const kindOrder: Record<SecurityCertificateKind, number> = {
  api_certificate: 0,
  identity: 1,
  authority: 2,
  trusted_authority: 3,
  pinned_peer: 4
};

export interface ExpiryStatus {
  label: string;
  tone: Tone;
}

export interface ListenerNotice {
  message: string;
  tone: Tone;
}

export interface ProfilePrompt {
  message: string;
  canSwitch: boolean;
}

export function isTokenRole(value: string): value is SecurityTokenRole {
  return (tokenRoles as readonly string[]).includes(value);
}

export function isCertificateKind(value: string): value is SecurityCertificateKind {
  return (importableKinds as readonly string[]).includes(value);
}

export function isRequestKind(value: string): value is SecurityRequestKind {
  return (requestKinds as readonly string[]).includes(value);
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

export function trustableAuthorities(certificates: readonly SecurityCertificate[]): SecurityCertificate[] {
  return certificates.filter(certificate => certificate.kind === "authority" || certificate.kind === "trusted_authority");
}

export function certificateHosts(certificate: SecurityCertificate): string[] {
  return [...certificate.dns_names, ...certificate.ip_addresses];
}

export function shortFingerprint(fingerprint: string): string {
  return fingerprint.slice(0, 16).replace(/(.{4})(?=.)/g, "$1:");
}

export function daysUntilExpiry(notAfter: string, now: Date): number {
  return Math.floor((new Date(notAfter).getTime() - now.getTime()) / dayMilliseconds);
}

export function expiryStatus(notAfter: string, now: Date): ExpiryStatus {
  const remainingDays = daysUntilExpiry(notAfter, now);
  if (remainingDays < 0) {
    return {label: "Expired", tone: "danger"};
  }
  if (remainingDays <= expiryWarningDays) {
    return {label: `Expires in ${remainingDays} d`, tone: "warning"};
  }
  return {label: new Date(notAfter).toLocaleDateString(), tone: "neutral"};
}

export function canRevealSecret(token: SecurityToken, target: SecurityTarget): boolean {
  return token.role === "cluster" && target.scope === "local";
}

export function canActivate(certificate: SecurityCertificate): boolean {
  return isActivatableKind(certificate.kind) && !certificate.active;
}

export function isActivatableKind(kind: SecurityCertificateKind): boolean {
  return kind === "identity" || kind === "api_certificate";
}

export function canIssue(certificate: SecurityCertificate, target: SecurityTarget): boolean {
  return certificate.kind === "authority" && target.scope === "local";
}

export function parseHosts(value: string): string[] {
  return value.split(/[\s,]+/).map(host => host.trim()).filter(host => host !== "");
}

export function listenerNotices(status: SecurityStatus | null): ListenerNotice[] {
  if (!status) {
    return [];
  }
  const notices = status.listeners.map(listener => listenerNotice(listener, status));
  if (status.api_certificate_active && status.listeners.some(listener => listener.name === "shared")) {
    notices.push({message: "The API certificate is not served while cluster peers share this port. Set cluster.bind in config.yaml to give peers their own port, then restart.", tone: "warning"});
  }
  return notices;
}

function listenerNotice(listener: SecurityListener, status: SecurityStatus): ListenerNotice {
  const port = `${listenerLabels[listener.name]} ${listener.bind}`;
  if (listener.restart_required) {
    return {message: `${port}: restart the router to switch between HTTP and HTTPS.`, tone: "warning"};
  }
  if (!listener.tls) {
    return {message: `${port}: plain HTTP. ${plainListenerAdvice[listener.name]}`, tone: "info"};
  }
  return {message: `${port}: HTTPS with ${servedCertificate(listener, status)}.`, tone: "success"};
}

const listenerLabels: Record<SecurityListener["name"], string> = {
  api: "API port",
  cluster: "Cluster port",
  shared: "Port"
};

const plainListenerAdvice: Record<SecurityListener["name"], string> = {
  api: "Import and activate an API certificate so clients connect over HTTPS.",
  cluster: "Activate a router identity so traffic between routers is encrypted.",
  shared: "Activate a router identity so traffic between routers is encrypted."
};

function servedCertificate(listener: SecurityListener, status: SecurityStatus): string {
  if (listener.name === "api") {
    return status.api_certificate_active ? "the API certificate" : "the router identity";
  }
  return listener.name === "cluster" ? "mutual TLS for cluster peers" : "the router identity and mutual TLS for cluster peers";
}

export function profilePrompt(status: SecurityStatus | null, certificates: readonly SecurityCertificate[]): ProfilePrompt | null {
  if (!status) {
    return null;
  }
  if (status.profile_restart_required) {
    return {message: "config.yaml now sets security.profile to secure. Restart the router so model and admin APIs require tokens.", canSwitch: false};
  }
  if (status.profile !== "trusted_lan" || certificates.length === 0) {
    return null;
  }
  const risk = "Certificates are configured, but security.profile is trusted_lan, so model and admin APIs accept requests without a token.";
  if (status.secure_profile_blocker) {
    return {message: `${risk} Before switching to secure: ${status.secure_profile_blocker}.`, canSwitch: false};
  }
  return {message: `${risk} Switch to the secure profile?`, canSwitch: true};
}

export function pemFilename(name: string, suffix: string): string {
  const safeName = name.trim().replace(/[^A-Za-z0-9._-]+/g, "_") || "certificate";
  return `${safeName}${suffix}`;
}

export function pemBlob(content: string): Blob {
  return new Blob([content], {type: "application/x-pem-file"});
}
