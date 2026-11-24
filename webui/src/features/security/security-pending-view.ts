import { SafeHTML, html } from "../../lib/safe-html";
import type { SecurityPendingKey } from "../../types";
import { badge } from "../../ui/badge";
import { button, buttonStrip } from "../../ui/buttons";
import { emptyTableRow } from "../../ui/empty-state";
import { kindLabel } from "./security-data";

const pendingKeyColumnCount = 5;

export function pendingKeyRowsMarkup(pendingKeys: readonly SecurityPendingKey[]): SafeHTML {
  if (pendingKeys.length === 0) {
    return emptyTableRow("No pending requests. Create one to have an outside CA sign a key that stays on the router.", pendingKeyColumnCount);
  }
  return html`${pendingKeys.map(pendingKeyRow)}`;
}

function pendingKeyRow(pendingKey: SecurityPendingKey): SafeHTML {
  const actions = [
    button("Download CSR", {"data-security-request-download": pendingKey.id}),
    button("Delete", {class: "danger", "data-security-request-delete": pendingKey.id, "data-security-name": pendingKey.name})
  ];
  return html`
    <tr>
      <td><strong>${pendingKey.name}</strong></td>
      <td>${badge(kindLabel(pendingKey.kind), "neutral")}</td>
      <td class="security-hosts">${[...pendingKey.dns_names, ...pendingKey.ip_addresses].join(", ")}</td>
      <td class="muted">${new Date(pendingKey.created_at).toLocaleString()}</td>
      <td class="security-actions">${buttonStrip(actions)}</td>
    </tr>
  `;
}
