import { SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import type { SecurityCertificate } from "../../types";
import { attributes } from "../../ui/attributes";
import { button } from "../../ui/buttons";
import { kindLabel } from "../security/security-data";
import { authorityDescription } from "./wizard-authority-data";
import type { WizardCertificateStatus } from "./wizard-certificate-data";

export function authorityControlsMarkup(candidates: readonly SecurityCertificate[], authority: SecurityCertificate | undefined, newName: string): SafeHTML {
  const picker = candidates.length > 0
    ? html`<label>CA<select data-wizard-authority data-operation-group="security">${candidates.map(candidate => html`<option${attributes({
      value: candidate.id,
      selected: candidate.id === authority?.id
    })}>${candidate.name} (${kindLabel(candidate.kind)})</option>`)}</select></label>`
    : emptyHTML;
  return html`
    <div class="security-wizard-controls">
      ${picker}
      <label>New local CA<input type="text" data-wizard-authority-name${attributes({value: newName})}></label>
      ${button("Create CA", {"data-wizard-create-authority": true, "data-operation-group": "security"})}
    </div>
    ${authority ? html`<p class="security-notice tone-info">${authorityDescription(authority)}</p>` : emptyHTML}
  `;
}

export function apiCertificateChoiceMarkup(status: WizardCertificateStatus, selected: boolean): SafeHTML {
  return html`<label class="security-wizard-choice"><input type="checkbox"${attributes({
    "data-wizard-api-node": status.nodeKey,
    checked: selected,
    "data-operation-group": "security"
  })}>Issue</label>`;
}

export function pendingRequestActions(status: WizardCertificateStatus): SafeHTML[] {
  if (!status.pendingKey) {
    return [];
  }
  return [
    button("Download request", {"data-wizard-request-download": status.pendingKey.id, "data-wizard-node": status.nodeKey}),
    button("Install signed", {"data-wizard-install-signed": status.nodeKey, "data-operation-group": "security"})
  ];
}
