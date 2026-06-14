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
  benchmark: {
    modelKey: "",
    type: "general",
    sections: ["runtime", "llm", "embed", "image", "voice", "music"],
    record: null,
    running: false,
    error: ""
  },
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
    openSections: [],
    sidebar: null
  },
  constructor: {
    lanes: emptyLanes(),
    targetNodes: emptyLaneTargets(),
    laneOptions: emptyLaneOptions(),
    backendMode: "kobold",
    backendTouched: false,
    options: {},
    fieldEditor: null,
    fieldPresets: [],
    showUsedAll: false,
    showOptionsAll: false
  },
  palettePayloads: {}
};
