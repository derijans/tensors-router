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
  routingGroups: null,
  router: null,
  nodes: {
    expanded: [],
    byNode: {}
  },
  models: {
    activeSubtab: "models",
    configNodeIDs: [],
    fileNodeIDs: [],
    modelSearch: "",
    enabledFilter: "all",
    backendFilter: "",
    capabilityFilter: "",
    fileSearch: "",
    fileRoleFilter: "",
    fileExtensionFilter: "",
    fileHashFilter: "all",
    initialized: false
  },
  benchmark: {
    modelKey: "",
    type: "general",
    sections: ["runtime", "llm", "embed", "image", "voice", "music"],
    record: null,
    running: false,
    error: ""
  },
  analytics: {
    query: {
      period: "24h"
    },
    data: null,
    loading: false,
    error: ""
  },
  loadCaptures: {
    query: {node_ids: [], status: "", kind: "", backend: ""},
    data: null,
    attempts: [],
    nextCursor: "",
    detail: null,
    output: [],
    outputCursor: 0,
    outputMore: false,
    loading: false,
    error: ""
  },
  webuis: {
    data: null,
    filter: "",
    loading: false,
    error: "",
    action: ""
  },
  downloads: {
    available: false,
    capabilities: null,
    nodeID: "",
    plan: null,
    selectedPlanFiles: [],
    library: null,
    search: [],
    nextCursor: "",
    filterTab: "main",
    filters: [],
    expandedFilterGroups: [],
    observedFilters: [],
    candidates: [],
    finderMessage: "",
    modelHandoff: null,
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
    cleanID: "",
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
    showOptionsAll: false,
    cleanSnapshot: ""
  },
  operations: {},
  conversionWarnings: {},
  acceptedConversionSignature: "",
  palettePayloads: {}
};
