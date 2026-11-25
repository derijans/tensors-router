import { downloadBlob } from "../../lib/download";
import { pemBlob, pemFilename } from "../security/security-data";
import { selectSecurityNode } from "../security/security";
import { openInstallSignedForm } from "../security/security-dialog";
import { wizardState } from "./wizard-state";

export function downloadWizardRequest(nodeKey: string, pendingKeyID: string): void {
  const report = wizardState.reports.find(candidate => candidate.node.key === nodeKey);
  const pendingKey = report?.summary?.pending_keys.find(key => key.id === pendingKeyID);
  if (pendingKey) {
    downloadBlob(pemBlob(pendingKey.request_pem), pemFilename(`${report?.node.label ?? "router"}-${pendingKey.name}`, ".csr.pem"));
  }
}

export async function installWizardSignedCertificate(nodeKey: string): Promise<void> {
  await selectSecurityNode(nodeKey);
  openInstallSignedForm();
}
