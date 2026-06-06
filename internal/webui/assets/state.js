export const state = {
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

export function emptyLanes() {
  return {
    text: null,
    image: null,
    embeddings: null
  };
}

export function emptyLaneTargets() {
  return {
    text: "",
    image: "",
    embeddings: ""
  };
}
