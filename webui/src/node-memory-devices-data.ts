import { formatGigabytes } from "./node-memory-data";
import type { NodeMemory, NodeMemoryDevice } from "./types";

export interface DeviceMemoryRow {
  name: string;
  detail: string;
  usedPercent: number | null;
  figure: string;
}

const vendorLabels: Record<string, string> = {
  nvidia: "NVIDIA",
  amd: "AMD",
  intel: "Intel",
  apple: "Apple",
  virtual: "Virtual",
  other: "Other"
};

export function deviceMemoryRows(memory: NodeMemory | null | undefined): DeviceMemoryRow[] {
  const devices = memory?.devices ?? [];
  const rows = devices.filter(device => device.dedicated_total_mb > 0).map(deviceRow);
  const ram = memory?.system_ram;
  if (ram && ram.total_mb > 0 && (rows.length > 0 || memory?.kind === "vram")) {
    rows.push({
      name: "System RAM",
      detail: "host memory",
      usedPercent: percentOf(ram.used_mb, ram.total_mb),
      figure: `${formatGigabytes(ram.used_mb)} / ${formatGigabytes(ram.total_mb)}`
    });
  }
  return rows;
}

function deviceRow(device: NodeMemoryDevice): DeviceMemoryRow {
  const vendor = vendorLabels[device.vendor] ?? device.vendor;
  const shared = (device.shared_used_mb ?? 0) > 0 ? ` · ${formatGigabytes(device.shared_used_mb ?? 0)} shared` : "";
  const kind = device.unified ? "unified" : "dedicated";
  if (!device.usage_known) {
    return {
      name: device.name || vendor,
      detail: `${vendor} · ${kind} · usage not reported by ${device.source}`,
      usedPercent: null,
      figure: formatGigabytes(device.dedicated_total_mb)
    };
  }
  return {
    name: device.name || vendor,
    detail: `${vendor} · ${kind}${shared} · ${device.source}`,
    usedPercent: percentOf(device.dedicated_used_mb, device.dedicated_total_mb),
    figure: `${formatGigabytes(device.dedicated_used_mb)} / ${formatGigabytes(device.dedicated_total_mb)}`
  };
}

function percentOf(used: number, total: number): number {
  return Math.min(100, Math.max(0, (used / total) * 100));
}
