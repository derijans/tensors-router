import { SafeHTML, html } from "../../lib/safe-html";
import { button, dialogActions, primaryButton } from "../../ui/buttons";
import { labelledToggle } from "../../ui/form";
import { optionElement } from "../../ui/select";
import { importableKinds, kindLabel, roleLabel, tokenRoles } from "./security-data";
import { dialogFrame, formFrame, hostsField, nameField } from "./security-form-frame-view";

export function tokenFormMarkup(): SafeHTML {
  return formFrame("token", "Create token", html`
    ${nameField()}
    <label>Role<select name="role">${tokenRoles.map(role => optionElement(role, roleLabel(role)))}</select></label>
    <label>Existing value (optional)
      <input name="value" type="password" autocomplete="off" minlength="16">
    </label>
    <p class="dialog-note">Leave empty to generate a random token. Supply a value only to match a cluster token already used by other routers.</p>
  `, "Create");
}

export function generateIdentityFormMarkup(defaultHosts: readonly string[], subject: string): SafeHTML {
  return formFrame("generate", "Generate self-signed identity", html`
    ${nameField()}
    ${hostsField(defaultHosts)}
    ${labelledToggle({name: "activate"}, true, `Activate as the identity of ${subject}`)}
    <p class="dialog-note">Peers trust a self-signed identity once they import its certificate as a pinned peer.</p>
  `, "Generate");
}

export function importCertificateFormMarkup(): SafeHTML {
  return formFrame("import", "Import certificate", html`
    ${nameField()}
    <label>Kind<select name="kind">${importableKinds.map(kind => optionElement(kind, kindLabel(kind)))}</select></label>
    <label>Certificate PEM (identity: leaf first, then intermediates)
      <textarea name="certificate_pem" rows="6" required spellcheck="false"></textarea>
    </label>
    <label>Private key PEM (API certificate, router identity and local CA)
      <textarea name="private_key_pem" rows="4" spellcheck="false" autocomplete="off"></textarea>
    </label>
    ${labelledToggle({name: "activate"}, false, "Activate when the certificate is an API certificate or a router identity")}
    <p class="dialog-note">An API certificate is what API clients and browsers see, for example one signed by your pfSense CA.</p>
  `, "Import");
}

export function authorityFormMarkup(): SafeHTML {
  return formFrame("authority", "Create local certificate authority", html`
    ${nameField()}
    <p class="dialog-note">The CA key never leaves this router. Issue identities for other routers and import this CA there as a trusted CA.</p>
  `, "Create");
}

export function issueFormMarkup(authorityID: string, authorityName: string, defaultHosts: readonly string[]): SafeHTML {
  return formFrame("issue", `Issue identity from ${authorityName}`, html`
    <input type="hidden" name="authority_id" value="${authorityID}">
    ${nameField("Identity name")}
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
