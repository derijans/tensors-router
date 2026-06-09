import { beforeEach, describe, expect, it } from "vitest";
import { advancedLaneOptions } from "../constructor-options";
import { state } from "../state";
import { optionDefinition, testInventory } from "./factories";

describe("advanced cook option filtering", () => {
  beforeEach(() => {
    state.inventory = testInventory([
      optionDefinition("quiet", "bool", "runtime"),
      optionDefinition("model_param", "string", "llm"),
      optionDefinition("sdmodel", "string", "image")
    ], []);
  });

  it("preserves runtime and unknown options while filtering unrelated known lane options", () => {
    expect(advancedLaneOptions("text", {
      quiet: true,
      mystery_backend_key: "custom",
      model_param: "text.gguf",
      sdmodel: "image.safetensors"
    })).toEqual({
      quiet: true,
      mystery_backend_key: "custom",
      model_param: "text.gguf"
    });
  });
});
