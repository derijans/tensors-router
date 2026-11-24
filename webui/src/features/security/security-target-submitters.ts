import {
  createSecurityCertificateRequest,
  createSecurityToken,
  generateSecurityIdentity,
  installSignedSecurityCertificate
} from "../../api/security-target-api";
import type { SecurityTarget } from "../../types";
import { isRequestKind, isTokenRole, parseHosts } from "./security-data";
import { closeSecurityDialog, formChecked, formText, openSecurityDialog } from "./security-dialog-shell";
import type { SecurityFormSubmitter } from "./security-form-submitter";
import { secretRevealMarkup } from "./security-forms-view";
import { certificateRequestCreatedMarkup } from "./security-request-forms-view";
import { runSecurityChange } from "./security";

async function submitToken(values: FormData, target: SecurityTarget): Promise<void> {
  const role = formText(values, "role");
  if (!isTokenRole(role)) {
    throw new Error("Choose a token role.");
  }
  const request = {name: formText(values, "name"), role, value: formText(values, "value")};
  const created = await runSecurityChange(`Created token ${request.name}`, () => createSecurityToken(target, request));
  if (request.value !== "") {
    closeSecurityDialog();
    return;
  }
  openSecurityDialog(secretRevealMarkup("Token created", created.secret, "Copy this secret now. It is stored only as a hash and cannot be shown again."));
}

async function submitGenerate(values: FormData, target: SecurityTarget): Promise<void> {
  const request = {name: formText(values, "name"), hosts: parseHosts(formText(values, "hosts")), activate: formChecked(values, "activate")};
  await runSecurityChange(`Generated identity ${request.name}`, () => generateSecurityIdentity(target, request));
  closeSecurityDialog();
}

async function submitCertificateRequest(values: FormData, target: SecurityTarget): Promise<void> {
  const kind = formText(values, "kind");
  if (!isRequestKind(kind)) {
    throw new Error("Choose what the certificate is for.");
  }
  const request = {name: formText(values, "name"), kind, hosts: parseHosts(formText(values, "hosts"))};
  const pendingKey = await runSecurityChange(`Created certificate request ${request.name}`, () => createSecurityCertificateRequest(target, request));
  openSecurityDialog(certificateRequestCreatedMarkup(pendingKey.name), {request: {content: pendingKey.request_pem, name: pendingKey.name, suffix: ".csr.pem"}});
}

async function submitInstallSigned(values: FormData, target: SecurityTarget): Promise<void> {
  const request = {certificate_pem: formText(values, "certificate_pem"), activate: formChecked(values, "activate")};
  await runSecurityChange("Installed signed certificate", () => installSignedSecurityCertificate(target, request));
  closeSecurityDialog();
}

export const targetFormSubmitters = {
  token: submitToken,
  generate: submitGenerate,
  request: submitCertificateRequest,
  install: submitInstallSigned
} satisfies Record<string, SecurityFormSubmitter>;
