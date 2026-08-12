import { describe, expect, it } from "vitest";
import { fileExtensionOptions, filterInventoryFiles, filterInventoryModels, modelBackends } from "../model-inventory-data";
import type { FileRecord, Model } from "../types";
import { testModel } from "./factories";

function models(): Model[] {
  return [
    {...testModel("alpha"), node_id: "node-a", backend_mode: "kobold", has_image: true},
    {...testModel("beta"), node_id: "node-b", backend_mode: "llama_sdcpp", has_embeddings: true, disabled: true},
    {...testModel("gamma"), node_id: "node-b", backend_mode: "kobold", has_voice: true},
    {...testModel("delta"), node_id: "node-c", backend_mode: "vllm", has_embeddings: true, disabled: true}
  ];
}

describe("model inventory filters", () => {
  it("combines search, nodes, enabled state, backend, and capability", () => {
    const filtered = filterInventoryModels(models(), {
      query: "beta",
      nodeIDs: ["node-b"],
      enabled: "disabled",
      backend: "llama_sdcpp",
      capability: "embeddings"
    });
    expect(filtered.map(model => model.local_id)).toEqual(["beta"]);
  });

  it("treats omitted disabled as enabled", () => {
    expect(filterInventoryModels(models(), {query: "", nodeIDs: ["*"], enabled: "enabled", backend: "", capability: ""}).map(model => model.local_id)).toEqual(["alpha", "gamma"]);
  });

  it("includes and filters vLLM inventory records", () => {
    expect(modelBackends(models())).toEqual(["kobold", "llama_sdcpp", "vllm"]);
    expect(filterInventoryModels(models(), {query: "", nodeIDs: ["*"], enabled: "all", backend: "vllm", capability: "embeddings"}).map(model => model.local_id)).toEqual(["delta"]);
  });
});

describe("file inventory filters", () => {
  const files: FileRecord[] = [
    {path: "A:/same.gguf", basename: "same.gguf", extension: ".gguf", size: 1, modified: 0, node_id: "node-a", role: "llm", sha256: "abc"},
    {path: "B:/same.gguf", basename: "same.gguf", extension: "gguf", size: 2, modified: 0, node_id: "node-b", role: "image"},
    {path: "B:/voice.bin", basename: "voice.bin", extension: ".bin", size: 3, modified: 0, node_id: "node-b", role: "voice"}
  ];

  it("combines search, nodes, role, extension, and hash state", () => {
    const filtered = filterInventoryFiles(files, {query: "same", nodeIDs: ["node-b"], role: "image", extension: ".gguf", hash: "unhashed"});
    expect(filtered.map(file => file.path)).toEqual(["B:/same.gguf"]);
  });

  it("keeps duplicate filenames on separate nodes and normalizes extensions", () => {
    expect(filterInventoryFiles(files, {query: "same", nodeIDs: ["*"], role: "", extension: "", hash: "all"})).toHaveLength(2);
    expect(fileExtensionOptions(files)).toEqual([".bin", ".gguf"]);
  });
});
