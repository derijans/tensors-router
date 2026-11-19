import { formatGigabytes } from "./node-memory-data";
import type { ConfigMemoryFit, MemoryFitLevel, MemoryFitTier, NodeMemoryFit } from "./types";

export interface MemoryFitBadge {
  label: string;
  className: string;
  tooltip: string;
}

const levelLabels: Record<MemoryFitLevel, string> = {
  unknown: "not learned",
  green: "fits",
  yellow: "tight",
  light_amber: "small spill",
  deep_amber: "large spill",
  red: "won't fit"
};

const tierLabels: Record<MemoryFitTier, string> = {
  unknown: "no samples yet",
  measured: "measured on this node",
  context_scaled: "scaled from another context size",
  architecture: "estimated from models of the same architecture"
};

export function memoryFitFor(fits: Readonly<Record<string, NodeMemoryFit>>, nodeID: string, modelID: string): ConfigMemoryFit | null {
  return fits[nodeID]?.configs.find(config => config.model_id === modelID) ?? null;
}

export function memoryFitBadge(fit: ConfigMemoryFit): MemoryFitBadge {
  return {
    label: levelLabels[fit.level],
    className: `fit-badge fit-${fit.level.replace("_", "-")}`,
    tooltip: memoryFitTooltip(fit)
  };
}

function memoryFitTooltip(fit: ConfigMemoryFit): string {
  if (fit.unavailable) {
    return fit.unavailable;
  }
  if (fit.level === "unknown") {
    return "Load this config once on this node to learn its memory use.";
  }
  const lines = [
    `${tierLabels[fit.tier]}`,
    `Needs ${bandText(fit.vram.low_mb, fit.vram.high_mb)} ${fit.ram_only ? "memory" : "VRAM"} and ${bandText(fit.ram.low_mb, fit.ram.high_mb)} RAM`,
    fit.ram_only ? `Free RAM ${formatGigabytes(fit.ram_free_mb)}` : `Free VRAM ${formatGigabytes(fit.gpu_free_mb)} · free RAM ${formatGigabytes(fit.ram_free_mb)}`
  ];
  if ((fit.spill_mb ?? 0) > 0) {
    lines.push(`About ${formatGigabytes(fit.spill_mb ?? 0)} may spill into system RAM; without driver memory fallback this is an out-of-memory error.`);
  }
  if (fit.multi_gpu) {
    lines.push("Compared against all GPUs together; a split across them can still run out on one GPU.");
  }
  lines.push("A warning only: the load is never blocked.");
  return lines.join("\n");
}

function bandText(lowMB: number, highMB: number): string {
  return Math.round(lowMB) === Math.round(highMB) ? formatGigabytes(lowMB) : `${formatGigabytes(lowMB)}–${formatGigabytes(highMB)}`;
}
