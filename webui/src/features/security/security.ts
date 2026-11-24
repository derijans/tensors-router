import { setHTML } from "../../lib/safe-html";
import { errorMessage } from "../../lib/errors";
import { downloadBlob } from "../../lib/download";
import {
  activateSecurityCertificate,
  deleteSecurityCertificate,
  deleteSecurityToken,
  downloadSecurityCertificatePEM,
  getSecurityCertificates,
  getSecurityStatus,
  getSecurityTokens,
  switchSecurityProfileToSecure
} from "../../api/security-api";
import { state } from "../../state/app-state";
import { showStatus } from "../../ui/status-line";
import { confirmDestructive } from "../shell/dialogs";
import { listenerNotice, pemFilename, profilePrompt, sortedCertificates, sortedTokens } from "./security-data";
import { securityElements } from "./security-elements";
import { certificateRowsMarkup, tokenRowsMarkup } from "./security-view";

export async function loadSecurity(): Promise<void> {
  state.security.loading = true;
  state.security.error = "";
  renderSecurity();
  try {
    const [tokens, certificates, status] = await Promise.all([getSecurityTokens(), getSecurityCertificates(), getSecurityStatus()]);
    state.security.tokens = tokens.tokens;
    state.security.certificates = certificates.certificates;
    state.security.defaultHosts = certificates.default_hosts;
    state.security.status = status;
  } catch (error) {
    state.security.error = errorMessage(error);
  } finally {
    state.security.loading = false;
    renderSecurity();
  }
}

export function renderSecurity(): void {
  const notice = listenerNotice(state.security.status);
  securityElements.listenerNotice.textContent = notice.message;
  securityElements.listenerNotice.className = `security-notice tone-${notice.tone}`;
  renderProfilePrompt();
  showStatus(securityElements.status, state.security.error || state.security.action, state.security.error !== "");
  setHTML(securityElements.tokenRows, tokenRowsMarkup(sortedTokens(state.security.tokens)));
  setHTML(securityElements.certificateRows, certificateRowsMarkup(sortedCertificates(state.security.certificates), new Date()));
}

function renderProfilePrompt(): void {
  const prompt = profilePrompt(state.security.status, state.security.certificates);
  securityElements.profileNotice.hidden = prompt === null;
  securityElements.profileMessage.textContent = prompt?.message ?? "";
  securityElements.secureProfileButton.hidden = !prompt?.canSwitch;
}

export async function switchToSecureProfile(): Promise<void> {
  const consequences = "The router rewrites security.profile in its config.yaml. After a restart, model APIs need an inference token and admin APIs need an admin token, so set router.token in webui.yaml to an admin token.";
  if (!await confirmDestructive("Switch to secure profile?", consequences, "Switch to secure")) {
    return;
  }
  await runSecurityChange("Updated config.yaml.", switchSecurityProfileToSecure);
}

export async function runSecurityChange(action: string, change: () => Promise<unknown>): Promise<void> {
  state.security.error = "";
  try {
    await change();
    state.security.action = action;
  } catch (error) {
    state.security.error = errorMessage(error);
    throw error;
  } finally {
    await loadSecurity();
  }
}

export async function deleteToken(id: string, name: string): Promise<void> {
  if (!await confirmDestructive("Delete token?", `Clients and peers that use ${name} lose access immediately.`, "Delete token")) {
    return;
  }
  await runSecurityChange(`Deleted token ${name}`, () => deleteSecurityToken(id));
}

export async function activateCertificate(id: string, name: string): Promise<void> {
  if (!await confirmDestructive("Activate identity?", `${name} becomes the certificate this router presents. Peers must trust it before they can reach this router.`, "Activate")) {
    return;
  }
  await runSecurityChange(`Activated ${name}`, () => activateSecurityCertificate(id));
}

export async function deleteCertificate(id: string, name: string, active: boolean): Promise<void> {
  const message = active
    ? `${name} is the active identity. After a restart the router serves plain HTTP until another identity is activated.`
    : `${name} is removed. Peers that relied on it are no longer trusted or able to connect.`;
  if (!await confirmDestructive("Delete certificate?", message, "Delete certificate")) {
    return;
  }
  await runSecurityChange(`Deleted ${name}`, () => deleteSecurityCertificate(id, active));
}

export async function downloadCertificate(id: string, name: string): Promise<void> {
  downloadBlob(await downloadSecurityCertificatePEM(id), pemFilename(name, ".pem"));
}
