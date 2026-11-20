import { type HTMLValue, SafeHTML, html } from "../lib/safe-html";
import { type Attributes, attributes } from "./attributes";

export function button(label: HTMLValue, values: Attributes = {}): SafeHTML {
  return html`<button type="button"${attributes(values)}>${label}</button>`;
}

export function primaryButton(label: HTMLValue, values: Attributes = {}): SafeHTML {
  return button(label, {class: "primary", ...values});
}

export function iconButton(label: HTMLValue, accessibleName: string, values: Attributes = {}): SafeHTML {
  return button(label, {class: "icon-button", "aria-label": accessibleName, ...values});
}

export function closeButton(accessibleName: string, values: Attributes = {}): SafeHTML {
  return iconButton("×", accessibleName, values);
}

export function linkButton(label: HTMLValue, values: Attributes = {}): SafeHTML {
  return button(label, {class: "link-button", ...values});
}

export function buttonStrip(buttons: readonly SafeHTML[]): SafeHTML {
  return html`<div class="button-strip">${buttons}</div>`;
}

export function dialogActions(buttons: readonly SafeHTML[]): SafeHTML {
  return html`<div class="dialog-actions">${buttons}</div>`;
}
