import type { LaneKind } from "./types";

export const laneKinds: LaneKind[] = ["text", "image", "embeddings"];

export function isLaneKind(value: string | undefined): value is LaneKind {
  return laneKinds.includes(value as LaneKind);
}
