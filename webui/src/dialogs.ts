import { elements } from "./elements";
import { escapeAttribute, escapeHTML, optionValueLabel } from "./utils";
import type { ConversionWarning } from "./types";

interface DialogChoice {
  label: string;
  value: string;
  danger?: boolean;
}

interface DialogRequest {
  title: string;
  message: string;
  details?: string;
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
  const rows = warnings.map(warning => `
    <tr>
      <td><code>${escapeHTML(warning.field)}</code></td>
      <td>${escapeHTML(warning.original)}</td>
      <td>${escapeHTML(optionValueLabel(warning.proposed))}</td>
      <td>${escapeHTML(warning.reason)}</td>
    </tr>
  `).join("");
  const choice = await showDialog({
    title: "Review lossy conversions",
    message: "These inputs will change meaning when saved.",
    details: `
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
  elements.safetyDialogBody.innerHTML = `
    <h2>${escapeHTML(request.title)}</h2>
    <p class="dialog-warning">${escapeHTML(request.message)}</p>
    ${request.details || ""}
    <div class="dialog-actions">
      ${request.choices.map(choice => `<button type="button" data-dialog-choice="${escapeAttribute(choice.value)}"${choice.danger ? " class=\"danger\"" : ""}>${escapeHTML(choice.label)}</button>`).join("")}
    </div>
  `;
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
