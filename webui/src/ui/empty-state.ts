import { type HTMLValue, SafeHTML, html } from "../lib/safe-html";
import { classNames } from "./attributes";

export function emptyState(message: HTMLValue, extraClass = ""): SafeHTML {
  return html`<div class="${classNames("empty-state", extraClass)}">${message}</div>`;
}

export function emptyCard(message: HTMLValue): SafeHTML {
  return emptyState(message, "card");
}

export function emptyTableRow(message: HTMLValue, columnCount: number, cellClass = "empty-state"): SafeHTML {
  return html`<tr><td class="${cellClass}" colspan="${columnCount}">${message}</td></tr>`;
}

export function listOrFallback(items: readonly SafeHTML[], fallback: SafeHTML): SafeHTML {
  return items.length > 0 ? html`${items}` : fallback;
}
