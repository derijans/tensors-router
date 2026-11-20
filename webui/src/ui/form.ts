import { type HTMLValue, SafeHTML, html } from "../lib/safe-html";
import { type Attributes, attributes } from "./attributes";

export function checkbox(values: Attributes, checked: boolean): SafeHTML {
  return html`<input type="checkbox"${attributes({...values, checked})}>`;
}

export function toggleRow(input: SafeHTML, label: HTMLValue): SafeHTML {
  return html`<label class="toggle-row">${input}${label}</label>`;
}

export function labelledToggle(values: Attributes, checked: boolean, label: HTMLValue): SafeHTML {
  return toggleRow(checkbox(values, checked), html`<span>${label}</span>`);
}
