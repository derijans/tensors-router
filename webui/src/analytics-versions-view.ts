import { SafeHTML, html } from "./safe-html";
import type { AnalyticsVersionUsage } from "./types";
import { badge } from "./markup-primitives";

export function renderVersionRows(versions: AnalyticsVersionUsage[]): SafeHTML {
  return html`${versions.map(usage => html`<tr>
    <td>${usage.node_id}</td>
    <td>${usage.router_version ? usage.router_version : badge("unknown (recorded before version stamping)", "warning")}</td>
    <td>${new Date(usage.first_seen).toLocaleString()}</td>
    <td>${new Date(usage.last_seen).toLocaleString()}</td>
    <td>${usage.event_count}</td>
  </tr>`)}`;
}
