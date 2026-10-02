import { describe, expect, it } from "vitest";
import { buildSearchIndex, searchEntries, type SearchSources } from "../search/search-index";
import type { Model, NodeInventory } from "../types";

function model(publicID: string, nodeID = "atlas"): Model {
  return {public_id: publicID, local_id: publicID, node_id: nodeID, filename: `${publicID}.gguf`, backend_mode: "kobold"} as Model;
}

function sources(overrides: Partial<SearchSources> = {}): SearchSources {
  return {sections: [], nodes: [], models: [], recipes: [], webuis: [], ...overrides};
}

describe("global search", () => {
  it("ranks a prefix match above a word match above a substring above a detail match", () => {
    const index = buildSearchIndex(sources({models: [
      model("mistral-qwen"),
      model("deepqwen"),
      model("qwen3-32b"),
      model("llama", "qwen-node")
    ]}));

    expect(searchEntries(index, "qwen").map(entry => entry.label)).toEqual(["qwen3-32b", "mistral-qwen", "deepqwen", "llama"]);
  });

  it("groups results by kind with sections first and caps each kind", () => {
    const index = buildSearchIndex(sources({
      sections: [{tab: "nodes", label: "Nodes"}],
      nodes: [{node_id: "node-a", role: "slave", backend_mode: "kobold"} as NodeInventory],
      models: Array.from({length: 7}, (_, index) => model(`node-model-${index}`))
    }));
    const results = searchEntries(index, "node");

    expect(results.map(entry => entry.kind)).toEqual(["section", "node", "model", "model", "model", "model", "model"]);
    expect(results[0]?.target).toEqual({kind: "section", tab: "nodes"});
    expect(results[1]?.target).toEqual({kind: "node", nodeID: "node-a"});
  });

  it("returns nothing for a blank query", () => {
    expect(searchEntries(buildSearchIndex(sources({models: [model("qwen")]})), "  ")).toEqual([]);
  });
});
