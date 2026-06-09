import { laneMetadata } from "./constants";
import { optionDefinition } from "./data";
import type { LaneKind, Options } from "./types";

export function advancedLaneOptions(lane: LaneKind, options: Options): Options {
  const section = laneSection(lane);
  const result: Options = {};
  for (const [key, value] of Object.entries(options)) {
    const definition = optionDefinition(key);
    if (!definition || definition.section === "runtime" || definition.section === section) {
      result[key] = value;
    }
  }
  return result;
}

export function laneSection(lane: LaneKind): string {
  return laneMetadata[lane].section;
}
