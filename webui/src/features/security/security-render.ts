import { emptyHTML, setHTML } from "../../lib/safe-html";
import { state } from "../../state/app-state";
import type { SecuritySummary, SecurityTarget } from "../../types";
import { showStatus } from "../../ui/status-line";
import { apiCertificateNotice } from "./security-api-certificate-data";
import { listenerNotices, profilePrompt, sortedCertificates, sortedTokens } from "./security-data";
import { securityElements } from "./security-elements";
import { securityNodeOptions } from "./security-node-options-data";
import { pendingKeyRowsMarkup } from "./security-pending-view";
import { currentSecurityTarget, displayedSecuritySummary } from "./security-selection";
import { apiCertificateNoticeMarkup, securityNodeOptionsMarkup } from "./security-target-view";
import { certificateRowsMarkup, listenerNoticesMarkup, tokenRowsMarkup } from "./security-view";

const localOnlyControls = [securityElements.importCertificateButton, securityElements.createAuthorityButton];
const nodeOnlyControls = [securityElements.pushClusterTokenButton, securityElements.issueFromMasterButton, securityElements.trustMasterButton];

export function renderSecurity(): void {
  const target = currentSecurityTarget();
  const summary = displayedSecuritySummary();
  const now = new Date();
  renderNodePicker(now);
  localOnlyControls.forEach(control => { control.hidden = target.scope !== "local"; });
  nodeOnlyControls.forEach(control => { control.hidden = target.scope !== "node"; });
  setHTML(securityElements.listenerNotices, listenerNoticesMarkup(listenerNotices(summary?.status ?? null)));
  setHTML(securityElements.apiCertificateNotice, target.scope === "node" && summary ? apiCertificateNoticeMarkup(apiCertificateNotice(summary.api_certificate, now)) : emptyHTML);
  renderProfilePrompt(target, summary);
  showStatus(securityElements.status, state.security.error || state.security.action, state.security.error !== "");
  renderTables(target, summary, now);
}

function renderNodePicker(now: Date): void {
  const options = securityNodeOptions(state.inventory, now);
  securityElements.nodePicker.hidden = options.length === 0;
  setHTML(securityElements.nodeSelect, securityNodeOptionsMarkup(options, state.security.nodeID));
}

function renderProfilePrompt(target: SecurityTarget, summary: SecuritySummary | null): void {
  const prompt = target.scope === "local" && summary ? profilePrompt(summary.status, summary.certificates) : null;
  securityElements.profileNotice.hidden = prompt === null;
  securityElements.profileMessage.textContent = prompt?.message ?? "";
  securityElements.secureProfileButton.hidden = !prompt?.canSwitch;
}

function renderTables(target: SecurityTarget, summary: SecuritySummary | null, now: Date): void {
  if (!summary) {
    setHTML(securityElements.tokenRows, emptyHTML);
    setHTML(securityElements.certificateRows, emptyHTML);
    setHTML(securityElements.pendingKeyRows, emptyHTML);
    return;
  }
  setHTML(securityElements.tokenRows, tokenRowsMarkup(sortedTokens(summary.tokens), target));
  setHTML(securityElements.certificateRows, certificateRowsMarkup(sortedCertificates(summary.certificates), target, now));
  setHTML(securityElements.pendingKeyRows, pendingKeyRowsMarkup(summary.pending_keys));
}
