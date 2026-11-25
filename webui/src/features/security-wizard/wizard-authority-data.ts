import type { SecurityCertificate } from "../../types";

export const defaultAuthorityName = "cluster-ca";

export function authorityCandidates(certificates: readonly SecurityCertificate[]): SecurityCertificate[] {
  return certificates.filter(certificate => certificate.kind === "authority" || certificate.kind === "trusted_authority");
}

export function preferredAuthorityID(certificates: readonly SecurityCertificate[], chosenID: string): string {
  const candidates = authorityCandidates(certificates);
  if (candidates.some(candidate => candidate.id === chosenID)) {
    return chosenID;
  }
  const activeIdentity = certificates.find(certificate => certificate.kind === "identity" && certificate.active);
  const identityIssuer = activeIdentity ? candidates.find(candidate => issuedBy(activeIdentity, candidate)) : undefined;
  return identityIssuer?.id ?? candidates.find(signsOnMaster)?.id ?? candidates[0]?.id ?? "";
}

export function signsOnMaster(authority: SecurityCertificate): boolean {
  return authority.kind === "authority";
}

export function issuedBy(certificate: SecurityCertificate, authority: SecurityCertificate): boolean {
  return certificate.issuer === authority.subject && certificate.fingerprint !== authority.fingerprint;
}

export function holdsAuthority(certificates: readonly SecurityCertificate[], authority: SecurityCertificate): boolean {
  return authorityCandidates(certificates).some(certificate => certificate.fingerprint === authority.fingerprint);
}

export function authorityDescription(authority: SecurityCertificate): string {
  return signsOnMaster(authority)
    ? `Local CA ${authority.name}: the master signs every router's request with it.`
    : `Trusted CA ${authority.name}: its key is not on the master, so routers create requests that you sign with that CA and install here.`;
}
