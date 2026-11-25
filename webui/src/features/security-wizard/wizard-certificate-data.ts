import type { SecurityCertificate, SecurityPendingKey, SecurityRequestKind, SecuritySummary } from "../../types";
import { issuedBy, signsOnMaster } from "./wizard-authority-data";
import type { WizardNodeReport } from "./wizard-nodes-data";
import { type WizardNodeStatus, nodeStatus, unreachableStatus } from "./wizard-status-data";

export interface WizardCertificateStatus extends WizardNodeStatus {
  pendingKey?: SecurityPendingKey;
}

export function activeCertificate(summary: SecuritySummary, kind: SecurityRequestKind): SecurityCertificate | undefined {
  return summary.certificates.find(certificate => certificate.kind === kind && certificate.active);
}

export function pendingRequest(summary: SecuritySummary, kind: SecurityRequestKind): SecurityPendingKey | undefined {
  return summary.pending_keys.find(key => key.kind === kind);
}

export function unexpired(certificate: SecurityCertificate, now: Date): boolean {
  return new Date(certificate.not_after).getTime() > now.getTime();
}

export function identityStatuses(reports: readonly WizardNodeReport[], authority: SecurityCertificate | undefined, now: Date): WizardCertificateStatus[] {
  return reports.map(report => certificateStatus(report, authority, "identity", true, now));
}

export function defaultAPICertificateSelection(summary: SecuritySummary | null): boolean {
  return summary !== null && !summary.api_certificate.present;
}

export function apiCertificateStatuses(
  reports: readonly WizardNodeReport[],
  authority: SecurityCertificate | undefined,
  selected: (report: WizardNodeReport) => boolean,
  now: Date
): WizardCertificateStatus[] {
  return reports.map(report => certificateStatus(report, authority, "api_certificate", selected(report), now));
}

function certificateStatus(
  report: WizardNodeReport,
  authority: SecurityCertificate | undefined,
  kind: SecurityRequestKind,
  selected: boolean,
  now: Date
): WizardCertificateStatus {
  const summary = report.summary;
  if (!summary) {
    return unreachableStatus(report);
  }
  const active = activeCertificate(summary, kind);
  if (authority && active && issuedBy(active, authority) && unexpired(active, now)) {
    return nodeStatus(report, "done", `${active.name} from ${authority.name} is active, valid until ${new Date(active.not_after).toLocaleDateString()}`);
  }
  if (!selected) {
    return active
      ? nodeStatus(report, "done", `Keeps ${active.name} (${active.source}).`)
      : nodeStatus(report, "skipped", "No API certificate: the API port serves the identity under the secure profile and plain HTTP under trusted_lan.");
  }
  if (!authority) {
    return nodeStatus(report, "blocked", "Choose a CA in step 2.");
  }
  const pendingKey = pendingRequest(summary, kind);
  if (pendingKey && !signsOnMaster(authority)) {
    return {...nodeStatus(report, "waiting", `Request ${pendingKey.name} waits for a certificate signed by ${authority.name}.`), pendingKey};
  }
  return nodeStatus(report, "todo", plannedChange(active, authority, signsOnMaster(authority)));
}

function plannedChange(active: SecurityCertificate | undefined, authority: SecurityCertificate, signsHere: boolean): string {
  const action = signsHere ? `Issues a certificate from ${authority.name} and activates it` : `Creates a request for ${authority.name} to sign`;
  return active ? `${action}; it replaces ${active.name}.` : `${action}.`;
}
