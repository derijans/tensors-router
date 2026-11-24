import { SafeHTML, html } from "../../lib/safe-html";
import type { SecurityCertificate, SecurityTarget, SecurityToken } from "../../types";
import { badge, badgeRow } from "../../ui/badge";
import { button, buttonStrip } from "../../ui/buttons";
import { emptyTableRow } from "../../ui/empty-state";
import {
  canActivate,
  canIssue,
  type ListenerNotice,
  canRevealSecret,
  certificateHosts,
  expiryStatus,
  kindLabel,
  roleLabel,
  shortFingerprint
} from "./security-data";

const tokenColumnCount = 5;
const certificateColumnCount = 6;

export function listenerNoticesMarkup(notices: readonly ListenerNotice[]): SafeHTML {
  return html`${notices.map(notice => html`<p class="security-notice tone-${notice.tone}">${notice.message}</p>`)}`;
}

export function tokenRowsMarkup(tokens: readonly SecurityToken[], target: SecurityTarget): SafeHTML {
  if (tokens.length === 0) {
    return emptyTableRow("No tokens yet. Create one so clients and peers can authenticate.", tokenColumnCount);
  }
  return html`${tokens.map(token => tokenRow(token, target))}`;
}

function tokenRow(token: SecurityToken, target: SecurityTarget): SafeHTML {
  const actions = [
    canRevealSecret(token, target) ? button("Show", {"data-security-token-reveal": token.id}) : null,
    button("Delete", {class: "danger", "data-security-token-delete": token.id, "data-security-name": token.name})
  ].filter(action => action !== null);
  return html`
    <tr>
      <td><strong>${token.name}</strong></td>
      <td>${badge(roleLabel(token.role), token.role === "admin" ? "accent" : "neutral")}</td>
      <td class="muted">${token.source}</td>
      <td class="muted">${new Date(token.created_at).toLocaleString()}</td>
      <td class="security-actions">${buttonStrip(actions)}</td>
    </tr>
  `;
}

export function certificateRowsMarkup(certificates: readonly SecurityCertificate[], target: SecurityTarget, now: Date): SafeHTML {
  if (certificates.length === 0) {
    return emptyTableRow("No certificates yet. Generate an identity or install one signed by your CA.", certificateColumnCount);
  }
  return html`${certificates.map(certificate => certificateRow(certificate, target, now))}`;
}

function certificateRow(certificate: SecurityCertificate, target: SecurityTarget, now: Date): SafeHTML {
  const expiry = expiryStatus(certificate.not_after, now);
  return html`
    <tr>
      <td>
        <strong>${certificate.name}</strong>
        <div class="muted security-subject">${certificate.subject}</div>
      </td>
      <td>${badgeRow([badge(kindLabel(certificate.kind), "neutral"), certificate.active ? badge("Active", "success") : null])}</td>
      <td class="security-hosts">${certificateHosts(certificate).join(", ")}</td>
      <td><code title="${certificate.fingerprint}">${shortFingerprint(certificate.fingerprint)}</code></td>
      <td>${badge(expiry.label, expiry.tone)}</td>
      <td class="security-actions">${buttonStrip(certificateActions(certificate, target))}</td>
    </tr>
  `;
}

function certificateActions(certificate: SecurityCertificate, target: SecurityTarget): SafeHTML[] {
  const actions: SafeHTML[] = [];
  if (canActivate(certificate)) {
    actions.push(button("Activate", {"data-security-certificate-activate": certificate.id, "data-security-name": certificate.name}));
  }
  if (canIssue(certificate, target)) {
    actions.push(button("Issue", {"data-security-authority-issue": certificate.id, "data-security-name": certificate.name}));
  }
  actions.push(button("Download", {"data-security-certificate-download": certificate.id, "data-security-name": certificate.name}));
  actions.push(button("Delete", {
    class: "danger",
    "data-security-certificate-delete": certificate.id,
    "data-security-name": certificate.name,
    "data-security-active": certificate.active
  }));
  return actions;
}
