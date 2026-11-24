import {
  createSecurityAuthority,
  importSecurityCertificate,
  installIssuedSecurityIdentity,
  issueSecurityIdentity
} from "../../api/security-api";
import type { SecurityIssueRequest } from "../../types";
import { isActivatableKind, isCertificateKind, parseHosts } from "./security-data";
import { closeSecurityDialog, formChecked, formText, openSecurityDialog } from "./security-dialog-shell";
import type { SecurityFormSubmitter } from "./security-form-submitter";
import { issuedIdentityMarkup } from "./security-forms-view";
import { runSecurityChange } from "./security";

async function submitImport(values: FormData): Promise<void> {
  const kind = formText(values, "kind");
  if (!isCertificateKind(kind)) {
    throw new Error("Choose a certificate kind.");
  }
  const request = {
    name: formText(values, "name"),
    kind,
    certificate_pem: formText(values, "certificate_pem"),
    private_key_pem: formText(values, "private_key_pem"),
    activate: formChecked(values, "activate") && isActivatableKind(kind)
  };
  await runSecurityChange(`Imported ${request.name}`, () => importSecurityCertificate(request));
  closeSecurityDialog();
}

async function submitAuthority(values: FormData): Promise<void> {
  const name = formText(values, "name");
  await runSecurityChange(`Created certificate authority ${name}`, () => createSecurityAuthority({name}));
  closeSecurityDialog();
}

async function submitIssue(values: FormData): Promise<void> {
  const authorityID = formText(values, "authority_id");
  const request: SecurityIssueRequest = {
    name: formText(values, "name"),
    hosts: parseHosts(formText(values, "hosts")),
    install_locally: formChecked(values, "install_locally"),
    activate: formChecked(values, "activate")
  };
  if (request.install_locally) {
    await runSecurityChange(`Installed identity ${request.name}`, () => installIssuedSecurityIdentity(authorityID, request));
    closeSecurityDialog();
    return;
  }
  if (request.activate) {
    throw new Error("Activation needs the identity installed on this router.");
  }
  if (request.hosts.length === 0) {
    throw new Error("List the hosts of the router that will use this identity.");
  }
  const identity = await issueSecurityIdentity(authorityID, request);
  openSecurityDialog(issuedIdentityMarkup(request.name), {
    certificate: {content: identity.certificate_pem, name: request.name, suffix: ".crt.pem"},
    key: {content: identity.private_key_pem, name: request.name, suffix: ".key.pem"}
  });
}

export const localFormSubmitters = {
  import: submitImport,
  authority: submitAuthority,
  issue: submitIssue
} satisfies Record<string, SecurityFormSubmitter>;
