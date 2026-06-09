import { describe, expect, it } from "vitest";
import { emptyComparableValue, parseOptionInput } from "../utils";
import { optionDefinition } from "./factories";

describe("option parsing", () => {
  it("preserves current bool, number, json, and string behavior", () => {
    expect(parseOptionInput(optionDefinition("enabled", "bool"), "yes")).toBe(true);
    expect(parseOptionInput(optionDefinition("enabled", "bool"), "false")).toBe(false);
    expect(parseOptionInput(optionDefinition("threads", "number"), "12")).toBe(12);
    expect(parseOptionInput(optionDefinition("threads", "number"), "nope")).toBe(0);
    expect(parseOptionInput(optionDefinition("overridekv", "json"), "")).toEqual({});
    expect(parseOptionInput(optionDefinition("overridekv", "json"), "[\"a\",2,true,null]")).toEqual(["a", 2, true, null]);
    expect(parseOptionInput(optionDefinition("overridekv", "json"), "not json")).toBe("not json");
    expect(parseOptionInput(optionDefinition("model_param", "string"), "custom.gguf")).toBe("custom.gguf");
  });
});

describe("comparison emptiness", () => {
  it("ignores null, empty string, empty arrays, empty objects, and missing values", () => {
    expect(emptyComparableValue(null)).toBe(true);
    expect(emptyComparableValue("")).toBe(true);
    expect(emptyComparableValue(["", null, []])).toBe(true);
    expect(emptyComparableValue({})).toBe(true);
    expect(emptyComparableValue(undefined)).toBe(true);
    expect(emptyComparableValue("model.gguf")).toBe(false);
  });
});
