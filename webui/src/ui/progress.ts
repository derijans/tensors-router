import { SafeHTML, html } from "../lib/safe-html";
import { type Attributes, attributes } from "./attributes";

export function progressBar(completed: number, total: number, values: Attributes = {}): SafeHTML {
  const max = Math.max(total, 1);
  const value = Math.min(Math.max(completed, 0), max);
  return html`<progress${attributes(values)} value="${value}" max="${max}"></progress>`;
}

export function indeterminateProgress(values: Attributes = {}): SafeHTML {
  return html`<progress${attributes(values)}></progress>`;
}
