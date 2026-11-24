import { type SafeHTML, emptyHTML, setHTML } from "../../lib/safe-html";
import { errorMessage } from "../../lib/errors";
import { downloadBlob } from "../../lib/download";
import {
  createSecurityAuthority,
  createSecurityToken,
  generateSecurityIdentity,
  importSecurityCertificate,
  installIssuedSecurityIdentity,
  issueSecurityIdentity,
  revealClusterTokenSecret
} from "../../api/security-api";
import type { SecurityIssueRequest, SecurityIssuedIdentity } from "../../types";
import { state } from "../../state/app-state";
import { showStatus } from "../../ui/status-line";
import { isCertificateKind, isTokenRole, parseHosts, pemFilename } from "./security-data";
import { securityElements } from "./security-elements";
import {
  type SecurityFormName,
  authorityFormMarkup,
  isSecurityFormName,
  generateIdentityFormMarkup,
  importCertificateFormMarkup,
  issueFormMarkup,
  issuedIdentityMarkup,
  secretRevealMarkup,
  tokenFormMarkup
} from "./security-forms-view";
import { runSecurityChange } from "./security";

interface IssuedDownload {
  name: string;
  identity: SecurityIssuedIdentity;
}

let issuingAuthorityID = "";
let issuedDownload: IssuedDownload | null = null;

function openDialog(content: SafeHTML): void {
  setHTML(securityElements.dialogBody, content);
  showStatus(securityElements.dialogStatus, "", false);
  if (!securityElements.dialog.open) {
    securityElements.dialog.showModal();
  }
}

export function closeSecurityDialog(): void {
  issuedDownload = null;
  setHTML(securityElements.dialogBody, emptyHTML);
  securityElements.dialog.close();
}

export function openTokenForm(): void {
  openDialog(tokenFormMarkup());
}

export function openGenerateIdentityForm(): void {
  openDialog(generateIdentityFormMarkup(state.security.defaultHosts));
}

export function openImportCertificateForm(): void {
  openDialog(importCertificateFormMarkup());
}

export function openAuthorityForm(): void {
  openDialog(authorityFormMarkup());
}

export function openIssueForm(authorityID: string, authorityName: string): void {
  issuingAuthorityID = authorityID;
  openDialog(issueFormMarkup(authorityName, state.security.defaultHosts));
}

export async function revealClusterSecret(id: string): Promise<void> {
  const response = await revealClusterTokenSecret(id);
  openDialog(secretRevealMarkup("Cluster token", response.secret, "Every router in the cluster must hold this same value as a cluster token."));
}

export async function copyDisplayedSecret(): Promise<void> {
  const field = securityElements.dialogBody.querySelector<HTMLInputElement>("[data-security-secret]");
  if (field) {
    await navigator.clipboard.writeText(field.value);
    showStatus(securityElements.dialogStatus, "Copied.", false);
  }
}

export function downloadIssuedPart(part: string): void {
  if (!issuedDownload) {
    return;
  }
  const isKey = part === "key";
  const content = isKey ? issuedDownload.identity.private_key_pem : issuedDownload.identity.certificate_pem;
  downloadBlob(new Blob([content], {type: "application/x-pem-file"}), pemFilename(issuedDownload.name, isKey ? ".key.pem" : ".crt.pem"));
}

export async function submitSecurityForm(form: HTMLFormElement): Promise<void> {
  const name = form.dataset.securityForm;
  if (!isSecurityFormName(name)) {
    return;
  }
  try {
    await formSubmitters[name](new FormData(form));
  } catch (error) {
    showStatus(securityElements.dialogStatus, errorMessage(error), true);
  }
}

function text(values: FormData, field: string): string {
  const value = values.get(field);
  return typeof value === "string" ? value.trim() : "";
}

function checked(values: FormData, field: string): boolean {
  return values.get(field) !== null;
}

const formSubmitters: Record<SecurityFormName, (values: FormData) => Promise<void>> = {
  token: submitToken,
  generate: submitGenerate,
  import: submitImport,
  authority: submitAuthority,
  issue: submitIssue
};

async function submitToken(values: FormData): Promise<void> {
  const role = text(values, "role");
  if (!isTokenRole(role)) {
    throw new Error("Choose a token role.");
  }
  const request = {name: text(values, "name"), role, value: text(values, "value")};
  let secret = "";
  await runSecurityChange(`Created token ${request.name}`, async () => {
    secret = (await createSecurityToken(request)).secret;
  });
  if (request.value !== "") {
    closeSecurityDialog();
    return;
  }
  openDialog(secretRevealMarkup("Token created", secret, "Copy this secret now. It is stored only as a hash and cannot be shown again."));
}

async function submitGenerate(values: FormData): Promise<void> {
  const request = {name: text(values, "name"), hosts: parseHosts(text(values, "hosts")), activate: checked(values, "activate")};
  await runSecurityChange(`Generated identity ${request.name}`, () => generateSecurityIdentity(request));
  closeSecurityDialog();
}

async function submitImport(values: FormData): Promise<void> {
  const kind = text(values, "kind");
  if (!isCertificateKind(kind)) {
    throw new Error("Choose a certificate kind.");
  }
  const request = {
    name: text(values, "name"),
    kind,
    certificate_pem: text(values, "certificate_pem"),
    private_key_pem: text(values, "private_key_pem"),
    activate: checked(values, "activate") && kind === "identity"
  };
  await runSecurityChange(`Imported ${request.name}`, () => importSecurityCertificate(request));
  closeSecurityDialog();
}

async function submitAuthority(values: FormData): Promise<void> {
  const name = text(values, "name");
  await runSecurityChange(`Created certificate authority ${name}`, () => createSecurityAuthority({name}));
  closeSecurityDialog();
}

async function submitIssue(values: FormData): Promise<void> {
  const request: SecurityIssueRequest = {
    name: text(values, "name"),
    hosts: parseHosts(text(values, "hosts")),
    install_locally: checked(values, "install_locally"),
    activate: checked(values, "activate")
  };
  if (request.install_locally) {
    await runSecurityChange(`Installed identity ${request.name}`, () => installIssuedSecurityIdentity(issuingAuthorityID, request));
    closeSecurityDialog();
    return;
  }
  if (request.activate) {
    throw new Error("Activation needs the identity installed on this router.");
  }
  if (request.hosts.length === 0) {
    throw new Error("List the hosts of the router that will use this identity.");
  }
  const identity = await issueSecurityIdentity(issuingAuthorityID, request);
  issuedDownload = {name: request.name, identity};
  openDialog(issuedIdentityMarkup(request.name));
}
