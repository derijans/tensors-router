import { beforeEach, describe, expect, it } from "vitest";
import { state } from "../state";
import {
  comparisonClass,
  fieldChoices,
  fieldRenderContext,
  groupedFieldKeys,
  importedConfigNameStem,
  parseImportedOptions
} from "../simple-cook-data";
import { optionDefinition, testInventory, testModel } from "./factories";

describe("simple cook option data", () => {
  beforeEach(() => {
    state.simpleCook.nodeID = "node-a";
    state.simpleCook.configID = "active";
    state.simpleCook.fields = {};
    state.inventory = testInventory([], [], []);
  });

  it("keeps observed unknown options grouped under Other", () => {
    const known = optionDefinition("model_param", "string", "llm");
    const observed = optionDefinition("surprise_backend_key", "json", "other", false);
    const groups = groupedFieldKeys(
      {model_param: "model.gguf", surprise_backend_key: "custom"},
      key => key === "model_param" ? known : observed
    );
    expect(groups.find(group => group.section === "llm")?.keys).toContain("model_param");
    expect(groups.find(group => group.section === "other")?.keys).toContain("surprise_backend_key");
  });

  it("keeps custom observed values alongside catalog choices", () => {
    const definition = {
      ...optionDefinition("sampling_method", "string", "image"),
      choices: ["euler", "heun"]
    };
    state.inventory = testInventory([definition], [], [
      testModel("active", {sampling_method: "custom_sampler"})
    ]);
    const choices = fieldChoices("sampling_method", definition, fieldRenderContext());
    expect(choices).toEqual(["euler", "heun", "custom_sampler"]);
  });

  it("does not color empty comparable values as real differences", () => {
    state.simpleCook.fields = {sampling_method: ""};
    state.inventory = testInventory([], [], [
      testModel("active", {sampling_method: ""}),
      testModel("empty-null", {sampling_method: null}),
      testModel("empty-array", {sampling_method: []}),
      testModel("empty-object", {sampling_method: {}})
    ]);
    expect(comparisonClass("sampling_method", "image", fieldRenderContext())).toBe("compare-none");
  });

  it("offers music capability files for music option fields", () => {
    const definition = {
      ...optionDefinition("musicvae", "string", "music"),
      model_role: "music"
    };
    state.inventory = testInventory([definition], [], [
      {
        ...testModel("active"),
        capabilities: {
          music: {
            vae: "music-vae.gguf"
          }
        }
      }
    ]);
    expect(fieldChoices("musicvae", definition, fieldRenderContext())).toContain("music-vae.gguf");
  });
});

describe("imported KCPPS parsing", () => {
  it("accepts a JSON object as options", () => {
    expect(parseImportedOptions(`{"model_param":"model.gguf","gpulayers":99}`)).toEqual({
      model_param: "model.gguf",
      gpulayers: 99
    });
  });

  it("rejects malformed JSON", () => {
    expect(() => parseImportedOptions("{not json")).toThrow("not valid JSON");
  });

  it("rejects a JSON array", () => {
    expect(() => parseImportedOptions("[1,2,3]")).toThrow("expected a JSON object");
  });

  it("rejects a bare JSON scalar", () => {
    expect(() => parseImportedOptions("42")).toThrow("expected a JSON object");
  });

  it("derives a safe id stem from the filename, stripping the kcpps/json extension", () => {
    expect(importedConfigNameStem("My Model.kcpps")).toBe("My-Model");
    expect(importedConfigNameStem("config.json")).toBe("config");
    expect(importedConfigNameStem(".kcpps")).toBe("imported-config");
  });
});
