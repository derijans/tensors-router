import { type HTMLValue, SafeHTML, html } from "../../lib/safe-html";
import { button, closeButton, dialogActions, primaryButton } from "../../ui/buttons";
import { optionElement } from "../../ui/select";
import type { SecurityCertificate } from "../../types";
import { kindLabel } from "./security-data";

const securityFormNames = ["token", "generate", "import", "authority", "issue", "request", "install", "master-issue", "trust"] as const;

export type SecurityFormName = typeof securityFormNames[number];

export function isSecurityFormName(value: string | undefined): value is SecurityFormName {
  return (securityFormNames as readonly (string | undefined)[]).includes(value);
}

export function dialogFrame(title: string, content: HTMLValue): SafeHTML {
  return html`
    <header class="dialog-head">
      <h2>${title}</h2>
      ${closeButton("Close", {"data-security-dialog-close": true})}
    </header>
    ${content}
  `;
}

export function formFrame(name: SecurityFormName, title: string, fields: HTMLValue, submitLabel: string): SafeHTML {
  return dialogFrame(title, html`
    <form class="security-form" data-security-form="${name}">
      ${fields}
      ${dialogActions([button("Cancel", {"data-security-dialog-close": true}), primaryButton(submitLabel, {type: "submit"})])}
    </form>
  `);
}

export function nameField(label = "Name"): SafeHTML {
  return html`<label>${label}<input name="name" required maxlength="128" autocomplete="off"></label>`;
}

export function hostsField(defaultHosts: readonly string[]): SafeHTML {
  return html`
    <label>Hosts (names and IPs, comma or space separated)
      <textarea name="hosts" rows="2" placeholder="${defaultHosts.join(", ")}"></textarea>
    </label>
    <p class="dialog-note">Leave empty to use ${defaultHosts.join(", ") || "loopback addresses"}. Peers verify the host in their cluster URL against this list.</p>
  `;
}

export function certificateSelect(name: string, certificates: readonly SecurityCertificate[]): SafeHTML {
  return html`<select name="${name}" required>${certificates.map(certificate => optionElement(certificate.id, `${certificate.name} (${kindLabel(certificate.kind)})`))}</select>`;
}

export function noticeFrame(title: string, note: string): SafeHTML {
  return dialogFrame(title, html`
    <p class="dialog-note">${note}</p>
    ${dialogActions([primaryButton("Close", {"data-security-dialog-close": true})])}
  `);
}
