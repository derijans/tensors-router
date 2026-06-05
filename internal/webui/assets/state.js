export const state = {
  csrf: "",
  inventory: null,
  router: null,
  activeTab: "router",
  activeCookMode: "quick",
  activePalette: "configs",
  constructor: {
    lanes: emptyLanes(),
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
