import { SafeHTML, emptyHTML, html } from "./safe-html";
import { memoryFitBadge } from "./memory-fit-data";
import type { ConfigMemoryFit } from "./types";

export function renderMemoryFitBadge(fit: ConfigMemoryFit | null): SafeHTML {
  if (!fit) {
    return emptyHTML;
  }
  const badge = memoryFitBadge(fit);
  return html`<span class="${badge.className}" title="${badge.tooltip}">${badge.label}</span>`;
}
