import { SafeHTML, html, setHTML } from "./safe-html";
import { elements } from "./elements";
import { optionValueLabel } from "./utils";
import type { ConversionWarning } from "./types";

interface DialogChoice {
  label: string;
  value: string;
  danger?: boolean;
}

interface DialogRequest {
  title: string;
  message: string;
  details?: SafeHTML;
  choices: DialogChoice[];
}

let resolveDialog: ((value: string) => void) | null = null;

export function registerSafetyDialog(): void {
  elements.safetyDialog.addEventListener("cancel", event => {
    event.preventDefault();
    finishDialog("");
  });
  elements.safetyDialog.addEventListener("click", event => {
    const target = event.target;
    if (!(target instanceof HTMLElement)) {
      return;
    }
    const value = target.dataset.dialogChoice;
    if (value !== undefined) {
      finishDialog(value);
    }
  });
}

export async function confirmDestructive(title: string, message: string, confirmLabel: string): Promise<boolean> {
  const choice = await showDialog({
    title,
    message,
    choices: [
      {label: "Cancel", value: "cancel"},
      {label: confirmLabel, value: "confirm", danger: true}
    ]
  });
  return choice === "confirm";
}

export async function reviewConversions(warnings: ConversionWarning[]): Promise<boolean> {
  const rows = html`${warnings.map(warning => html`
    <tr>
      <td><code>${warning.field}</code></td>
      <td>${warning.original}</td>
      <td>${optionValueLabel(warning.proposed)}</td>
      <td>${warning.reason}</td>
    </tr>
  `)}`;
  const choice = await showDialog({
    title: "Review lossy conversions",
    message: "These inputs will change meaning when saved.",
    details: html`
      <table class="conversion-table">
        <thead><tr><th>Field</th><th>Original</th><th>Proposed</th><th>Reason</th></tr></thead>
        <tbody>${rows}</tbody>
      </table>
    `,
    choices: [
      {label: "Return to editing", value: "edit"},
      {label: "Accept conversions", value: "accept"}
    ]
  });
  return choice === "accept";
}

async function showDialog(request: DialogRequest): Promise<string> {
  if (resolveDialog) {
    finishDialog("");
  }
  setHTML(elements.safetyDialogBody, html`
    <h2>${request.title}</h2>
    <p class="dialog-warning">${request.message}</p>
    ${request.details}
    <div class="dialog-actions">
      ${request.choices.map(choice => html`<button type="button" data-dialog-choice="${choice.value}"${choice.danger ? html` class="danger"` : ""}>${choice.label}</button>`)}
    </div>
  `);
  elements.safetyDialog.showModal();
  return new Promise(resolve => {
    resolveDialog = resolve;
  });
}

function finishDialog(value: string): void {
  const resolve = resolveDialog;
  resolveDialog = null;
  if (elements.safetyDialog.open) {
    elements.safetyDialog.close();
  }
  resolve?.(value);
}
