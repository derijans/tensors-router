import { describe, expect, it } from "vitest";
import { emptyComparableValue, parseOptionInput } from "../utils";
import { optionDefinition } from "./factories";

describe("option parsing", () => {
  it("returns values with exact lossy conversion warnings", () => {
    expect(parseOptionInput(optionDefinition("enabled", "bool"), "false")).toEqual({value: false, warnings: []});
    expect(parseOptionInput(optionDefinition("threads", "number"), "12")).toEqual({value: 12, warnings: []});
    expect(parseOptionInput(optionDefinition("overridekv", "json"), "[\"a\",2,true,null]")).toEqual({value: ["a", 2, true, null], warnings: []});
    expect(parseOptionInput(optionDefinition("model_param", "string"), "custom.gguf")).toEqual({value: "custom.gguf", warnings: []});

    const boolean = parseOptionInput(optionDefinition("enabled", "bool"), "yes");
    expect(boolean.value).toBe(true);
    expect(boolean.warnings).toEqual([{field: "enabled", original: "yes", proposed: true, reason: "The value is converted to a boolean."}]);

    const number = parseOptionInput(optionDefinition("threads", "number"), "nope");
    expect(number.value).toBe(0);
    expect(number.warnings[0]?.original).toBe("nope");

    const blankJSON = parseOptionInput(optionDefinition("overridekv", "json"), "");
    expect(blankJSON.value).toEqual({});
    expect(blankJSON.warnings[0]?.proposed).toEqual({});

    const invalidJSON = parseOptionInput(optionDefinition("overridekv", "json"), "not json");
    expect(invalidJSON.value).toBe("not json");
    expect(invalidJSON.warnings[0]?.reason).toBe("Invalid JSON is kept as a string.");
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
