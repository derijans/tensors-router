import { SafeHTML, emptyHTML, html } from "./safe-html";
import { memoryBar } from "./chart-markup";
import { deviceMemoryRows, type DeviceMemoryRow } from "./node-memory-devices-data";
import type { NodeMemory } from "./types";

export function renderDeviceMemory(memory: NodeMemory | null | undefined): SafeHTML {
  const rows = deviceMemoryRows(memory);
  if (rows.length === 0) {
    return emptyHTML;
  }
  return html`<ul class="device-memory-list">${rows.map(renderDeviceRow)}</ul>`;
}

function renderDeviceRow(row: DeviceMemoryRow): SafeHTML {
  const bar = row.usedPercent === null
    ? emptyHTML
    : memoryBar([{accent: "lane-other", percent: row.usedPercent, label: `${row.name} ${row.figure}`}], `${row.name} ${row.figure}`);
  return html`
    <li class="device-memory-row">
      <div class="device-memory-head">
        <span class="device-memory-name">${row.name}</span>
        <b>${row.figure}</b>
      </div>
      <small class="muted">${row.detail}</small>
      ${bar}
    </li>
  `;
}
