import { SafeHTML, displayText, html } from "../lib/safe-html";

export function fact(label: string, value: unknown): SafeHTML {
  return html`<div class="fact"><dt>${label}</dt><dd>${displayText(value)}</dd></div>`;
}

export type FactEntry = readonly [label: string, value: unknown];

export function factGrid(facts: readonly SafeHTML[]): SafeHTML {
  return html`<dl class="fact-grid">${facts}</dl>`;
}

export function presentFactGrid(entries: readonly FactEntry[]): SafeHTML {
  return factGrid(entries.filter(([, value]) => displayText(value) !== "").map(([label, value]) => fact(label, value)));
}
