import { elementTarget } from "../../lib/dom";
import { runTask } from "../shell/tasks";
import { selectSecurityNode, switchToSecureProfile } from "./security";
import {
  activateCertificate,
  deleteCertificate,
  deleteCertificateRequest,
  deleteToken,
  downloadCertificate,
  downloadCertificateRequest
} from "./security-actions";
import {
  openAuthorityForm,
  openCertificateRequestForm,
  openGenerateIdentityForm,
  openImportCertificateForm,
  openInstallSignedForm,
  openIssueForm,
  openMasterIssueForm,
  openTokenForm,
  openTrustMasterForm,
  revealClusterSecret
} from "./security-dialog";
import { bindSecurityDialog, securityGroup } from "./security-dialog-bindings";
import { securityElements } from "./security-elements";
import { pushClusterTokenToNode } from "./security-node-actions";

export function bindSecurity(): void {
  securityElements.nodeSelect.addEventListener("change", () => {
    const nodeID = securityElements.nodeSelect.value;
    runTask(() => selectSecurityNode(nodeID), "security-node", securityGroup, "Loading security…");
  });
  securityElements.createTokenButton.addEventListener("click", openTokenForm);
  securityElements.pushClusterTokenButton.addEventListener("click", () => {
    runTask(pushClusterTokenToNode, "security-cluster-token", securityGroup, "Sending cluster token…");
  });
  securityElements.generateIdentityButton.addEventListener("click", openGenerateIdentityForm);
  securityElements.importCertificateButton.addEventListener("click", openImportCertificateForm);
  securityElements.createAuthorityButton.addEventListener("click", openAuthorityForm);
  securityElements.issueFromMasterButton.addEventListener("click", openMasterIssueForm);
  securityElements.trustMasterButton.addEventListener("click", openTrustMasterForm);
  securityElements.createRequestButton.addEventListener("click", openCertificateRequestForm);
  securityElements.installSignedButton.addEventListener("click", openInstallSignedForm);
  securityElements.secureProfileButton.addEventListener("click", () => {
    runTask(switchToSecureProfile, "security-profile", securityGroup, "Switching profile…");
  });
  securityElements.tokenRows.addEventListener("click", handleTokenAction);
  securityElements.certificateRows.addEventListener("click", handleCertificateAction);
  securityElements.pendingKeyRows.addEventListener("click", handlePendingKeyAction);
  bindSecurityDialog();
}

function clickedButtonData(event: Event): DOMStringMap {
  return elementTarget(event)?.closest<HTMLElement>("button")?.dataset ?? {};
}

function handleTokenAction(event: Event): void {
  const {securityTokenReveal, securityTokenDelete, securityName = ""} = clickedButtonData(event);
  if (securityTokenReveal) {
    runTask(() => revealClusterSecret(securityTokenReveal), `security-reveal-${securityTokenReveal}`, securityGroup, "Loading cluster token…");
  } else if (securityTokenDelete) {
    runTask(() => deleteToken(securityTokenDelete, securityName), `security-token-${securityTokenDelete}`, securityGroup, "Deleting token…");
  }
}

function handleCertificateAction(event: Event): void {
  const data = clickedButtonData(event);
  const name = data.securityName ?? "";
  if (data.securityCertificateActivate) {
    const id = data.securityCertificateActivate;
    runTask(() => activateCertificate(id, name), `security-certificate-${id}`, securityGroup, "Activating certificate…");
  } else if (data.securityAuthorityIssue) {
    openIssueForm(data.securityAuthorityIssue, name);
  } else if (data.securityCertificateDownload) {
    const id = data.securityCertificateDownload;
    runTask(() => downloadCertificate(id, name), `security-download-${id}`, securityGroup, "Downloading certificate…");
  } else if (data.securityCertificateDelete) {
    const id = data.securityCertificateDelete;
    runTask(() => deleteCertificate(id, name, data.securityActive !== undefined), `security-certificate-${id}`, securityGroup, "Deleting certificate…");
  }
}

function handlePendingKeyAction(event: Event): void {
  const {securityRequestDownload, securityRequestDelete, securityName = ""} = clickedButtonData(event);
  if (securityRequestDownload) {
    downloadCertificateRequest(securityRequestDownload);
  } else if (securityRequestDelete) {
    runTask(() => deleteCertificateRequest(securityRequestDelete, securityName), `security-request-${securityRequestDelete}`, securityGroup, "Deleting request…");
  }
}
