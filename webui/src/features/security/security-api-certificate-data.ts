import type { SecurityApiCertificateState, SecurityCertificateSource, Tone } from "../../types";
import { daysUntilExpiry, expiryWarningDays } from "./security-data";

export interface ApiCertificateWarning {
  label: string;
  tone: Tone;
}

export interface ApiCertificateNotice {
  message: string;
  tone: Tone;
}

interface ExpiryConcern {
  phrase: string;
  tone: Tone;
}

const sourceLabels: Record<SecurityCertificateSource, string> = {
  migrated: "imported from config",
  master: "issued by master",
  supplied: "imported",
  created: "generated on the router",
  issued: "issued by the router's CA"
};

export function certificateSourceLabel(source: SecurityCertificateSource | undefined): string {
  return source ? sourceLabels[source] : "source unknown";
}

function expiryConcern(notAfter: string, now: Date): ExpiryConcern | null {
  const remainingDays = daysUntilExpiry(notAfter, now);
  if (remainingDays < 0) {
    return {phrase: "expired", tone: "danger"};
  }
  if (remainingDays <= expiryWarningDays) {
    return {phrase: `expires in ${remainingDays} d`, tone: "warning"};
  }
  return null;
}

export function apiCertificateWarning(certificate: SecurityApiCertificateState | undefined, now: Date): ApiCertificateWarning | null {
  if (!certificate) {
    return null;
  }
  if (!certificate.present) {
    return {label: "No API certificate", tone: "warning"};
  }
  const concern = expiryConcern(certificate.not_after ?? "", now);
  return concern ? {label: `API certificate ${concern.phrase}`, tone: concern.tone} : null;
}

export function apiCertificateNotice(certificate: SecurityApiCertificateState, now: Date): ApiCertificateNotice {
  if (!certificate.present) {
    return {message: "No API certificate is active, so API clients see the router identity or plain HTTP.", tone: "warning"};
  }
  const notAfter = certificate.not_after ?? "";
  const concern = expiryConcern(notAfter, now);
  const validity = concern ? concern.phrase : `valid until ${new Date(notAfter).toLocaleDateString()}`;
  return {
    message: `API certificate ${certificate.name ?? ""}: ${certificateSourceLabel(certificate.source)}, ${validity}.`,
    tone: concern ? concern.tone : "success"
  };
}
