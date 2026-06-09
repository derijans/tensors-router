import type { AppState, LaneKind, Options, PaletteComponentPayload } from "./types";

export function emptyLanes(): Record<LaneKind, PaletteComponentPayload | null> {
  return {
    text: null,
    image: null,
    embeddings: null,
    voice: null,
    music: null
  };
}

export function emptyLaneTargets(): Record<LaneKind, string> {
  return {
    text: "",
    image: "",
    embeddings: "",
    voice: "",
    music: ""
  };
}

export function emptyLaneOptions(): Record<LaneKind, Options> {
  return {
    text: {},
    image: {},
    embeddings: {},
    voice: {},
    music: {}
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
    laneOptions: emptyLaneOptions(),
    options: {},
    fieldEditor: null,
    fieldPresets: [],
    showUsedAll: false,
    showOptionsAll: false
  },
  palettePayloads: {}
};
