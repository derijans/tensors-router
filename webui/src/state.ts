import type { AppState, LaneKind, PaletteComponentPayload } from "./types";

export function emptyLanes(): Record<LaneKind, PaletteComponentPayload | null> {
  return {
    text: null,
    image: null,
    embeddings: null
  };
}

export function emptyLaneTargets(): Record<LaneKind, string> {
  return {
    text: "",
    image: "",
    embeddings: ""
  };
}

export const state: AppState = {
  csrf: "",
  inventory: null,
  router: null,
  activeTab: "router",
  activeCookMode: "quick",
  activePalette: "configs",
  simpleCook: {
    nodeID: "",
    configID: "",
    fields: {},
    cleanFields: {},
    mode: "edit",
    fieldFilter: "",
    sidebar: null
  },
  constructor: {
    lanes: emptyLanes(),
    targetNodes: emptyLaneTargets(),
    options: {},
    showUsedAll: false,
    showOptionsAll: false
  },
  palettePayloads: {}
};
