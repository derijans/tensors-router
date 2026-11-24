import { elementTarget } from "../../lib/dom";
import { runTask } from "../shell/tasks";
import { activateCertificate, deleteCertificate, deleteToken, downloadCertificate, switchToSecureProfile } from "./security";
import {
  closeSecurityDialog,
  copyDisplayedSecret,
  downloadIssuedPart,
  openAuthorityForm,
  openGenerateIdentityForm,
  openImportCertificateForm,
  openIssueForm,
  openTokenForm,
  revealClusterSecret,
  submitSecurityForm
} from "./security-dialog";
import { securityElements } from "./security-elements";

const securityGroup = "security";

export function bindSecurity(): void {
  securityElements.createTokenButton.addEventListener("click", openTokenForm);
  securityElements.generateIdentityButton.addEventListener("click", openGenerateIdentityForm);
  securityElements.importCertificateButton.addEventListener("click", openImportCertificateForm);
  securityElements.createAuthorityButton.addEventListener("click", openAuthorityForm);
  securityElements.secureProfileButton.addEventListener("click", () => {
    runTask(switchToSecureProfile, "security-profile", securityGroup, "Switching profile…");
  });
  securityElements.tokenRows.addEventListener("click", handleTokenAction);
  securityElements.certificateRows.addEventListener("click", handleCertificateAction);
  bindSecurityDialog();
}

function handleTokenAction(event: Event): void {
  const target = elementTarget(event)?.closest<HTMLElement>("button");
  const {securityTokenReveal, securityTokenDelete, securityName = ""} = target?.dataset ?? {};
  if (securityTokenReveal) {
    runTask(() => revealClusterSecret(securityTokenReveal), `security-reveal-${securityTokenReveal}`, securityGroup, "Loading cluster token…");
  } else if (securityTokenDelete) {
    runTask(() => deleteToken(securityTokenDelete, securityName), `security-token-${securityTokenDelete}`, securityGroup, "Deleting token…");
  }
}

function handleCertificateAction(event: Event): void {
  const target = elementTarget(event)?.closest<HTMLElement>("button");
  const data = target?.dataset ?? {};
  const name = data.securityName ?? "";
  if (data.securityCertificateActivate) {
    const id = data.securityCertificateActivate;
    runTask(() => activateCertificate(id, name), `security-certificate-${id}`, securityGroup, "Activating identity…");
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

function bindSecurityDialog(): void {
  securityElements.dialog.addEventListener("cancel", event => {
    event.preventDefault();
    closeSecurityDialog();
  });
  securityElements.dialog.addEventListener("submit", event => {
    const form = elementTarget(event)?.closest("form");
    if (form) {
      event.preventDefault();
      runTask(() => submitSecurityForm(form), "security-dialog", securityGroup, "Saving…");
    }
  });
  securityElements.dialog.addEventListener("click", event => {
    const target = elementTarget(event)?.closest<HTMLElement>("button");
    if (!target) {
      return;
    }
    if (target.dataset.securityDialogClose !== undefined) {
      closeSecurityDialog();
    } else if (target.dataset.securityCopySecret !== undefined) {
      runTask(copyDisplayedSecret, "security-copy", securityGroup, "Copying…");
    } else if (target.dataset.securityDownload) {
      downloadIssuedPart(target.dataset.securityDownload);
    }
  });
}
