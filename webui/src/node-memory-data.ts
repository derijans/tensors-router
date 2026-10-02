import { laneAccent } from "./markup-primitives";
import type { LaneAccent, NodeMemoryKind, NodeState } from "./types";

export type MemorySegmentAccent = LaneAccent | "lane-lent";

export interface MemorySegment {
  accent: MemorySegmentAccent;
  percent: number;
  label: string;
}

export interface NodeMemorySummary {
  kind: NodeMemoryKind;
  usedMB: number;
  totalMB: number;
  segments: MemorySegment[];
}

interface MemoryShare {
  accent: MemorySegmentAccent;
  megabytes: number;
  label: string;
}

export function memoryKindLabel(kind: NodeMemoryKind): string {
  return kind === "vram" ? "VRAM" : "RAM";
}

export function gigabyteFigure(megabytes: number): string {
  const gigabytes = Math.max(0, megabytes) / 1024;
  return gigabytes >= 100 ? gigabytes.toFixed(0) : gigabytes.toFixed(1);
}

export function formatGigabytes(megabytes: number): string {
  return `${gigabyteFigure(megabytes)} GB`;
}

export function nodeMemorySummary(snapshot: NodeState | null | undefined): NodeMemorySummary | null {
  const memory = snapshot?.memory;
  if (!snapshot || !memory || memory.total_mb <= 0) {
    return null;
  }
  const runtimeShares: MemoryShare[] = snapshot.backends.flatMap(backend => backend.loaded_models
    .filter(row => (row.memory_estimate_mb ?? 0) > 0)
    .map(row => ({
      accent: row.borrowed ? "lane-lent" as const : laneAccent(row.lane),
      megabytes: row.memory_estimate_mb ?? 0,
      label: `${row.model_id} · ~${formatGigabytes(row.memory_estimate_mb ?? 0)}${row.borrowed ? " · lent" : ""}`
    })));
  const attributed = runtimeShares.reduce((sum, share) => sum + share.megabytes, 0);
  const usedMB = Math.min(memory.used_mb, memory.total_mb);
  const scale = attributed > usedMB && attributed > 0 ? usedMB / attributed : 1;
  const unattributed = Math.max(0, usedMB - attributed * scale);
  const shares: MemoryShare[] = [
    ...runtimeShares.map(share => ({...share, megabytes: share.megabytes * scale})),
    ...(unattributed > 0 ? [{accent: "lane-other" as const, megabytes: unattributed, label: `Other use · ${formatGigabytes(unattributed)}`}] : [])
  ];
  return {
    kind: memory.kind,
    usedMB,
    totalMB: memory.total_mb,
    segments: shares.map(share => ({accent: share.accent, percent: (share.megabytes / memory.total_mb) * 100, label: share.label}))
  };
}
