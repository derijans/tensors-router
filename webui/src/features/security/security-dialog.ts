import { revealClusterTokenSecret } from "../../api/security-api";
import { authorities, trustableAuthorities } from "./security-data";
import { openSecurityDialog } from "./security-dialog-shell";
import {
  authorityFormMarkup,
  generateIdentityFormMarkup,
  importCertificateFormMarkup,
  issueFormMarkup,
  secretRevealMarkup,
  tokenFormMarkup
} from "./security-forms-view";
import { certificateRequestFormMarkup, installSignedFormMarkup, masterIssueFormMarkup, trustMasterFormMarkup } from "./security-request-forms-view";
import { currentSecurityTarget, displayedDefaultHosts, localCertificates } from "./security-selection";
import { securityTargetSubject } from "./security-target-data";

export function openTokenForm(): void {
  openSecurityDialog(tokenFormMarkup());
}

export function openGenerateIdentityForm(): void {
  openSecurityDialog(generateIdentityFormMarkup(displayedDefaultHosts(), securityTargetSubject(currentSecurityTarget())));
}

export function openImportCertificateForm(): void {
  openSecurityDialog(importCertificateFormMarkup());
}

export function openAuthorityForm(): void {
  openSecurityDialog(authorityFormMarkup());
}

export function openIssueForm(authorityID: string, authorityName: string): void {
  openSecurityDialog(issueFormMarkup(authorityID, authorityName, displayedDefaultHosts()));
}

export function openCertificateRequestForm(): void {
  openSecurityDialog(certificateRequestFormMarkup(displayedDefaultHosts(), securityTargetSubject(currentSecurityTarget())));
}

export function openInstallSignedForm(): void {
  openSecurityDialog(installSignedFormMarkup(securityTargetSubject(currentSecurityTarget())));
}

export function openMasterIssueForm(): void {
  const target = currentSecurityTarget();
  if (target.scope === "node") {
    openSecurityDialog(masterIssueFormMarkup(target.nodeID, authorities(localCertificates()), displayedDefaultHosts()));
  }
}

export function openTrustMasterForm(): void {
  const target = currentSecurityTarget();
  if (target.scope === "node") {
    openSecurityDialog(trustMasterFormMarkup(target.nodeID, trustableAuthorities(localCertificates())));
  }
}

export async function revealClusterSecret(id: string): Promise<void> {
  const response = await revealClusterTokenSecret(id);
  openSecurityDialog(secretRevealMarkup("Cluster token", response.secret, "Every router in the cluster must hold this same value as a cluster token."));
}
