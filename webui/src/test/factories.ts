import type { InventoryResponse, Model, NodeInventory, OptionDefinition, Options } from "../types";

export function optionDefinition(key: string, valueType: string, section = "other", known = true): OptionDefinition {
  return {
    key,
    name: key,
    lane: section,
    section,
    value_type: valueType,
    backends: ["kobold"],
    known
  };
}

export function testModel(id: string, options: Options = {}): Model {
  return {
    public_id: id,
    local_id: id,
    filename: `${id}.kcpps`,
    created: 0,
    has_llm: true,
    has_image: false,
    has_embeddings: false,
    has_multimodal: false,
    model_hash: "",
    config_hash: "",
    capabilities: {},
    options,
    backend_mode: "kobold",
    source: "local",
    node_id: "node-a",
    available: true
  };
}

export function testNode(models: Model[] = []): NodeInventory {
  return {
    node_id: "node-a",
    source: "local",
    role: "standalone",
    backend_mode: "kobold",
    available: true,
    hardware: {
      max_threads: 8,
      gpu_backend: "unknown",
      gpu_count: 0
    },
    models,
    files: []
  };
}

export function testInventory(optionCatalog: OptionDefinition[], observedOptions: OptionDefinition[], models: Model[] = []): InventoryResponse {
  return {
    role: "standalone",
    node_id: "node-a",
    nodes: [testNode(models)],
    models,
    recipes: [],
    option_catalog: optionCatalog,
    observed_options: observedOptions
  };
}
