import { SafeHTML, html } from "../../lib/safe-html";
import { button, dialogActions, primaryButton } from "../../ui/buttons";
import { labelledToggle } from "../../ui/form";
import { optionElement } from "../../ui/select";
import type { SecurityCertificate } from "../../types";
import { kindLabel, requestKinds } from "./security-data";
import { certificateSelect, dialogFrame, formFrame, hostsField, nameField, noticeFrame } from "./security-form-frame-view";

function requestKindField(): SafeHTML {
  return html`<label>Kind<select name="kind">${requestKinds.map(kind => optionElement(kind, kindLabel(kind)))}</select></label>`;
}

export function certificateRequestFormMarkup(defaultHosts: readonly string[], subject: string): SafeHTML {
  return formFrame("request", "Create certificate request", html`
    ${nameField()}
    ${requestKindField()}
    ${hostsField(defaultHosts)}
    <p class="dialog-note">The private key is created on ${subject} and never leaves it. Sign the downloaded request with your CA, then install the signed certificate.</p>
  `, "Create request");
}

export function certificateRequestCreatedMarkup(name: string): SafeHTML {
  return dialogFrame(`Request ${name} created`, html`
    <p class="dialog-note">Sign this request with your CA, then use Install signed certificate. The request stays listed until a matching certificate is installed.</p>
    ${dialogActions([
      button("Download request", {"data-security-download": "request"}),
      primaryButton("Done", {"data-security-dialog-close": true})
    ])}
  `);
}

export function installSignedFormMarkup(subject: string): SafeHTML {
  return formFrame("install", "Install signed certificate", html`
    <label>Certificate PEM (leaf first, then intermediates)
      <textarea name="certificate_pem" rows="8" required spellcheck="false"></textarea>
    </label>
    ${labelledToggle({name: "activate"}, true, "Activate after installing")}
    <p class="dialog-note">The certificate must match a pending request on ${subject}; its key is taken from that request.</p>
  `, "Install");
}

export function masterIssueFormMarkup(nodeID: string, issuers: readonly SecurityCertificate[], defaultHosts: readonly string[]): SafeHTML {
  if (issuers.length === 0) {
    return noticeFrame("Issue from master CA", "This router has no certificate authority yet. Select This router and create one first.");
  }
  return formFrame("master-issue", `Issue certificate for ${nodeID}`, html`
    <label>Master CA${certificateSelect("authority_id", issuers)}</label>
    ${nameField()}
    ${requestKindField()}
    ${hostsField(defaultHosts)}
    ${labelledToggle({name: "activate"}, true, "Activate after installing")}
    <p class="dialog-note">${nodeID} creates the key and a request, this router signs it, and ${nodeID} installs the certificate. No private key leaves ${nodeID}.</p>
  `, "Issue");
}

export function trustMasterFormMarkup(nodeID: string, authorities: readonly SecurityCertificate[]): SafeHTML {
  if (authorities.length === 0) {
    return noticeFrame("Trust master CA", "This router holds no CA certificate to send. Select This router and create or import one first.");
  }
  return formFrame("trust", `Trust a master CA on ${nodeID}`, html`
    <label>CA certificate${certificateSelect("certificate_id", authorities)}</label>
    <p class="dialog-note">Only the public certificate is sent. ${nodeID} then accepts peers whose identity this CA signed.</p>
  `, "Trust");
}
