import { downloadBlob } from "../../lib/download";
import {
  activateSecurityCertificate,
  deleteSecurityCertificate,
  deleteSecurityCertificateRequest,
  deleteSecurityToken,
  downloadSecurityCertificatePEM
} from "../../api/security-target-api";
import { confirmDestructive } from "../shell/dialogs";
import { pemBlob, pemFilename } from "./security-data";
import { currentSecurityTarget, displayedSecuritySummary } from "./security-selection";
import { securityTargetSubject } from "./security-target-data";
import { runSecurityChange } from "./security";

export async function deleteToken(id: string, name: string): Promise<void> {
  const target = currentSecurityTarget();
  if (!await confirmDestructive("Delete token?", `Clients and peers that use ${name} on ${securityTargetSubject(target)} lose access immediately.`, "Delete token")) {
    return;
  }
  await runSecurityChange(`Deleted token ${name}`, () => deleteSecurityToken(target, id));
}

export async function activateCertificate(id: string, name: string): Promise<void> {
  const target = currentSecurityTarget();
  const message = `${name} becomes the certificate ${securityTargetSubject(target)} presents. Peers must trust it before they can reach it.`;
  if (!await confirmDestructive("Activate certificate?", message, "Activate")) {
    return;
  }
  await runSecurityChange(`Activated ${name}`, () => activateSecurityCertificate(target, id));
}

export async function deleteCertificate(id: string, name: string, active: boolean): Promise<void> {
  const target = currentSecurityTarget();
  const message = active
    ? `${name} is active on ${securityTargetSubject(target)}. After a restart its port falls back to another certificate or to plain HTTP until another one is activated.`
    : `${name} is removed. Peers or clients that relied on it can no longer connect through it.`;
  if (!await confirmDestructive("Delete certificate?", message, "Delete certificate")) {
    return;
  }
  await runSecurityChange(`Deleted ${name}`, () => deleteSecurityCertificate(target, id, active));
}

export async function downloadCertificate(id: string, name: string): Promise<void> {
  downloadBlob(await downloadSecurityCertificatePEM(currentSecurityTarget(), id), pemFilename(name, ".pem"));
}

export function downloadCertificateRequest(id: string): void {
  const pendingKey = displayedSecuritySummary()?.pending_keys.find(candidate => candidate.id === id);
  if (pendingKey) {
    downloadBlob(pemBlob(pendingKey.request_pem), pemFilename(pendingKey.name, ".csr.pem"));
  }
}

export async function deleteCertificateRequest(id: string, name: string): Promise<void> {
  const target = currentSecurityTarget();
  if (!await confirmDestructive("Delete certificate request?", `The private key of ${name} is destroyed on ${securityTargetSubject(target)}. A certificate signed for this request can no longer be installed.`, "Delete request")) {
    return;
  }
  await runSecurityChange(`Deleted request ${name}`, () => deleteSecurityCertificateRequest(target, id));
}
