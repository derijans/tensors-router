import { type HTMLValue, SafeHTML, html } from "../../lib/safe-html";
import { button, closeButton, dialogActions, primaryButton } from "../../ui/buttons";
import { labelledToggle } from "../../ui/form";
import { optionElement } from "../../ui/select";
import { importableKinds, kindLabel, roleLabel, tokenRoles } from "./security-data";

const securityFormNames = ["token", "generate", "import", "authority", "issue"] as const;

export type SecurityFormName = typeof securityFormNames[number];

export function isSecurityFormName(value: string | undefined): value is SecurityFormName {
  return (securityFormNames as readonly (string | undefined)[]).includes(value);
}

function dialogFrame(title: string, content: HTMLValue): SafeHTML {
  return html`
    <header class="dialog-head">
      <h2>${title}</h2>
      ${closeButton("Close", {"data-security-dialog-close": true})}
    </header>
    ${content}
  `;
}

function formFrame(name: SecurityFormName, title: string, fields: HTMLValue, submitLabel: string): SafeHTML {
  return dialogFrame(title, html`
    <form class="security-form" data-security-form="${name}">
      ${fields}
      ${dialogActions([button("Cancel", {"data-security-dialog-close": true}), primaryButton(submitLabel, {type: "submit"})])}
    </form>
  `);
}

function hostsField(defaultHosts: readonly string[]): SafeHTML {
  return html`
    <label>Hosts (names and IPs, comma or space separated)
      <textarea name="hosts" rows="2" placeholder="${defaultHosts.join(", ")}"></textarea>
    </label>
    <p class="dialog-note">Leave empty to use ${defaultHosts.join(", ") || "loopback addresses"}. Peers verify the host in their cluster URL against this list.</p>
  `;
}

export function tokenFormMarkup(): SafeHTML {
  return formFrame("token", "Create token", html`
    <label>Name<input name="name" required maxlength="128" autocomplete="off"></label>
    <label>Role<select name="role">${tokenRoles.map(role => optionElement(role, roleLabel(role)))}</select></label>
    <label>Existing value (optional)
      <input name="value" type="password" autocomplete="off" minlength="16">
    </label>
    <p class="dialog-note">Leave empty to generate a random token. Supply a value only to match a cluster token already used by other routers.</p>
  `, "Create");
}

export function generateIdentityFormMarkup(defaultHosts: readonly string[]): SafeHTML {
  return formFrame("generate", "Generate self-signed identity", html`
    <label>Name<input name="name" required maxlength="128" autocomplete="off"></label>
    ${hostsField(defaultHosts)}
    ${labelledToggle({name: "activate"}, true, "Activate as this router's identity")}
    <p class="dialog-note">Peers trust a self-signed identity once they import its certificate as a pinned peer.</p>
  `, "Generate");
}

export function importCertificateFormMarkup(): SafeHTML {
  return formFrame("import", "Import certificate", html`
    <label>Name<input name="name" required maxlength="128" autocomplete="off"></label>
    <label>Kind<select name="kind">${importableKinds.map(kind => optionElement(kind, kindLabel(kind)))}</select></label>
    <label>Certificate PEM (identity: leaf first, then intermediates)
      <textarea name="certificate_pem" rows="6" required spellcheck="false"></textarea>
    </label>
    <label>Private key PEM (router identity and local CA only)
      <textarea name="private_key_pem" rows="4" spellcheck="false" autocomplete="off"></textarea>
    </label>
    ${labelledToggle({name: "activate"}, false, "Activate when the certificate is a router identity")}
  `, "Import");
}

export function authorityFormMarkup(): SafeHTML {
  return formFrame("authority", "Create local certificate authority", html`
    <label>Name<input name="name" required maxlength="128" autocomplete="off"></label>
    <p class="dialog-note">The CA key never leaves this router. Issue identities for other routers and import this CA there as a trusted CA.</p>
  `, "Create");
}

export function issueFormMarkup(authorityName: string, defaultHosts: readonly string[]): SafeHTML {
  return formFrame("issue", `Issue identity from ${authorityName}`, html`
    <label>Identity name<input name="name" required maxlength="128" autocomplete="off"></label>
    ${hostsField(defaultHosts)}
    ${labelledToggle({name: "install_locally"}, false, "Install on this router instead of downloading")}
    ${labelledToggle({name: "activate"}, false, "Activate after installing")}
  `, "Issue");
}

export function secretRevealMarkup(title: string, secret: string, note: string): SafeHTML {
  return dialogFrame(title, html`
    <p class="dialog-warning">${note}</p>
    <label>Secret<input class="security-secret" value="${secret}" readonly data-security-secret></label>
    ${dialogActions([button("Copy", {"data-security-copy-secret": true}), primaryButton("Done", {"data-security-dialog-close": true})])}
  `);
}

export function issuedIdentityMarkup(name: string): SafeHTML {
  return dialogFrame(`Identity ${name} issued`, html`
    <p class="dialog-warning">Download both files now. The private key is not stored on this router.</p>
    <p class="dialog-note">On the target router import the certificate and key as a router identity, and import this CA as a trusted CA.</p>
    ${dialogActions([
      button("Download certificate", {"data-security-download": "certificate"}),
      button("Download key", {"data-security-download": "key"}),
      primaryButton("Done", {"data-security-dialog-close": true})
    ])}
  `);
}
