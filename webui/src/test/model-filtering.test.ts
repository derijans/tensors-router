import { afterEach, describe, expect, it } from "vitest";
import { filteredFiles, filteredModels, visibleModelsForResolution } from "../data";
import { state } from "../state";

describe("model filtering", () => {
  afterEach(() => {
    state.inventory = null;
  });

  it("combines text filtering with explicit multi-node selection", () => {
    state.inventory = {
      models: [
        {local_id: "alpha", public_id: "alpha", filename: "alpha.kcpps", node_id: "node-a"},
        {local_id: "beta", public_id: "beta", filename: "beta.kcpps", node_id: "node-b"},
        {local_id: "gamma", public_id: "gamma", filename: "gamma.kcpps", node_id: "node-c"}
      ]
    } as typeof state.inventory;
    expect(filteredModels("", ["node-a", "node-c"]).map(model => model.local_id)).toEqual(["alpha", "gamma"]);
    expect(filteredModels("beta", ["*"]).map(model => model.local_id)).toEqual(["beta"]);
  });

  it("submits every visible config regardless of asset state", () => {
    state.inventory = {
      models: [
        {local_id: "ready", public_id: "ready", filename: "ready.kcpps", node_id: "node-a", asset_state: "ready"},
        {local_id: "unresolved", public_id: "unresolved", filename: "unresolved.kcpps", node_id: "node-a", asset_state: "unresolved"},
        {local_id: "failed", public_id: "failed", filename: "failed.kcpps", node_id: "node-b", asset_state: "failed"}
      ]
    } as typeof state.inventory;
    expect(visibleModelsForResolution("", ["node-a"]).map(model => model.local_id)).toEqual(["ready", "unresolved"]);
  });

  it("keeps same-named files from selected nodes as separate rows", () => {
    state.inventory = {
      nodes: [
        {node_id: "node-a", files: [{node_id: "node-a", path: "A:/model.gguf", basename: "model.gguf"}]},
        {node_id: "node-b", files: [{node_id: "node-b", path: "B:/model.gguf", basename: "model.gguf"}]}
      ]
    } as typeof state.inventory;
    expect(filteredFiles("", ["*"]).map(file => `${file.node_id}:${file.path}`)).toEqual(["node-a:A:/model.gguf", "node-b:B:/model.gguf"]);
    expect(filteredFiles("model", ["node-b"]).map(file => file.node_id)).toEqual(["node-b"]);
  });
});
