import { SafeHTML, html } from "../lib/safe-html";
import type { ValidationIssue } from "../types";

export function renderIssue(item: ValidationIssue): SafeHTML {
  return html`
    <div class="issue ${item.severity === "error" ? "error" : ""}">
      <strong>${item.severity} / ${item.code}</strong>
      <span>${item.message}</span>
    </div>
  `;
}
