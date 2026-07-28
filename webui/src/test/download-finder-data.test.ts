import { describe, expect, it } from "vitest";
import { normalizeModelHash, normalizeParameterRange, parseOfficialHFURL, splitSearchFilters } from "../download-finder-data";
import { hfFilterCatalog, hfFilterCatalogVersion } from "../hf-filter-catalog";

describe("download finder data", () => {
  it("separates dedicated Hugging Face filter parameters", () => {
    expect(splitSearchFilters(["gguf", "app:llama.cpp", "provider:hf-inference", "dataset:allenai/c4", "inference:true", "gguf"])).toEqual({
      filters: ["gguf"],
      apps: ["llama.cpp"],
      providers: ["hf-inference"],
      datasets: ["allenai/c4"],
      inference: true
    });
  });

  it("normalizes parameter ranges and hashes", () => {
    expect(normalizeParameterRange("6", "128")).toBe("min:6B,max:128B");
    expect(normalizeParameterRange("", "7.5")).toBe("min:0B,max:7.5B");
    expect(() => normalizeParameterRange("-1", "")).toThrow();
    expect(normalizeModelHash("A".repeat(64))).toBe("a".repeat(64));
    expect(() => normalizeModelHash("abc")).toThrow();
  });

  it("accepts only official Hugging Face URLs", () => {
    expect(parseOfficialHFURL("https://huggingface.co/owner/model/resolve/0123456789abcdef/model.gguf")).toEqual({repository: "owner/model", revision: "0123456789abcdef", file: "model.gguf"});
    expect(parseOfficialHFURL("hf://owner/model@0123456789abcdef/model.gguf").repository).toBe("owner/model");
    expect(() => parseOfficialHFURL("https://example.com/owner/model")).toThrow();
    expect(() => parseOfficialHFURL("https://huggingface.co/owner/model/resolve/main/../secret")).toThrow();
  });

  it("contains the versioned grouped filter catalog", () => {
    expect(hfFilterCatalogVersion).toBeGreaterThan(0);
    expect(Object.keys(hfFilterCatalog)).toEqual(["main", "tasks", "libraries", "languages", "licenses", "other"]);
    expect(hfFilterCatalog.tasks?.map(group => group.id)).toEqual(expect.arrayContaining(["multimodal", "vision", "nlp", "audio", "tabular", "reinforcement"]));
  });
});
