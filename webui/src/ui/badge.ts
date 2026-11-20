import { type HTMLValue, SafeHTML, displayText, emptyHTML, html } from "../lib/safe-html";
import type { Accent, Tone } from "../types";
import { type Attributes, attributes, classNames } from "./attributes";

export function accentClass(accent: Accent): string {
  return accent.startsWith("lane-") ? `lane-badge ${accent}` : `tone-${accent}`;
}

export function badge(label: unknown, accent: Accent): SafeHTML {
  const value = displayText(label).trim();
  if (!value) {
    return emptyHTML;
  }
  return html`<span class="badge ${accentClass(accent)}">${value}</span>`;
}

export function badgeButton(label: HTMLValue, accent: Accent, values: Attributes = {}, extraClass = ""): SafeHTML {
  return html`<button type="button"${attributes({class: classNames("badge", accentClass(accent), extraClass), ...values})}>${label}</button>`;
}

export function statusDot(label: HTMLValue, tone: Tone): SafeHTML {
  return html`<span class="status-dot tone-${tone}">${label}</span>`;
}

export function badgeRow(content: HTMLValue): SafeHTML {
  return html`<div class="badge-row">${content}</div>`;
}
