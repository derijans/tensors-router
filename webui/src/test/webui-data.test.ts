import { describe, expect, it } from "vitest";
import {
  filteredWebUIEntries,
  groupWebUIs,
  webUIDialogData,
  webUIOpenStatus
} from "../webui-data";
import type { WebUICompatibleModel, WebUIEntry } from "../types";

describe("WebUI data helpers", () => {
  it("groups by node and filters by entry or model fields", () => {
    const entries = [
      webUIEntry({id: "b:sd", name: "sd-server UI", backend: "stable-diffusion.cpp", lane: "image", node_id: "b", compatible_models: [compatibleModel("dream")]}),
      webUIEntry({id: "a:kobold", name: "KoboldCpp Lite", node_id: "a", compatible_models: [compatibleModel("chat")]}),
      webUIEntry({id: "a:llama", name: "llama-server UI", backend: "llama.cpp", node_id: "a", compatible_models: [compatibleModel("instruct")]})
    ];

    expect(groupWebUIs(entries).map(group => group.nodeID)).toEqual(["a", "b"]);
    expect(groupWebUIs(entries)[0]?.entries.map(entry => entry.name)).toEqual(["KoboldCpp Lite", "llama-server UI"]);
    expect(filteredWebUIEntries(entries, "dream").map(entry => entry.id)).toEqual(["b:sd"]);
    expect(filteredWebUIEntries(entries, "llama.cpp").map(entry => entry.id)).toEqual(["a:llama"]);
  });

  it("reports blocked and openable states from server-owned flags", () => {
    expect(webUIOpenStatus(webUIEntry({enabled: false}))).toEqual({openable: false, reason: "disabled"});
    expect(webUIOpenStatus(webUIEntry({enabled: true, active: false, requires_loaded_model: true, can_open_without_model: false}))).toEqual({openable: false, reason: "needs_model"});
    expect(webUIOpenStatus(webUIEntry({enabled: true, active: true}))).toEqual({openable: true, reason: ""});
    expect(webUIOpenStatus(webUIEntry({enabled: true, active: false, requires_loaded_model: false}))).toEqual({openable: true, reason: ""});
  });

  it("builds dialog data with active model first", () => {
    const entry = webUIEntry({
      enabled: false,
      compatible_models: [
        compatibleModel("zeta"),
        compatibleModel("alpha", true)
      ]
    });

    const data = webUIDialogData(entry);
    expect(data.canEnable).toBe(true);
    expect(data.canLoad).toBe(true);
    expect(data.message).toBe("Enable this WebUI before opening.");
    expect(data.models.map(model => model.id)).toEqual(["alpha", "zeta"]);
  });
});

function webUIEntry(overrides: Partial<WebUIEntry> = {}): WebUIEntry {
  return {
    id: "node:kobold",
    name: "KoboldCpp Lite",
    backend: "koboldcpp",
    backend_mode: "kobold",
    lane: "text",
    url: "https://ui.example.test/",
    node_id: "node",
    enabled: true,
    active: false,
    requires_loaded_model: true,
    can_open_without_model: false,
    compatible_models: [compatibleModel("chat")],
    ...overrides
  };
}

function compatibleModel(id: string, active = false): WebUICompatibleModel {
  return {
    id,
    model_id: id,
    filename: `${id}.kcpps`,
    node_id: "node",
    backend_mode: "kobold",
    active
  };
}
