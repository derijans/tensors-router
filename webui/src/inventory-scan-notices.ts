import { SafeHTML, html } from "./safe-html";
import type { NodeInventory } from "./types";

export function inventoryScanNotices(nodes: NodeInventory[]): SafeHTML {
  return html`${nodes.filter(node => node.error).map(inventoryScanNotice)}`;
}

function inventoryScanNotice(node: NodeInventory): SafeHTML {
  return html`<div class="inventory-notice error-text">${node.node_id || node.node_url || "unknown node"}: ${node.error || "scan failed"}</div>`;
}
