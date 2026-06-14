import { beforeEach, describe, expect, it } from "vitest";
import { advancedLaneOptions } from "../constructor-options";
import { state } from "../state";
import { optionDefinition, testInventory } from "./factories";

describe("advanced cook option filtering", () => {
  beforeEach(() => {
    state.inventory = testInventory([
      optionDefinition("backend_mode", "string", "runtime"),
      optionDefinition("quiet", "bool", "runtime"),
      optionDefinition("model_param", "string", "llm"),
      optionDefinition("sdmodel", "string", "image"),
      optionDefinition("whispermodel", "string", "voice"),
      optionDefinition("musicdiffusion", "string", "music")
    ], []);
  });

  it("preserves runtime and unknown options while filtering unrelated known lane options", () => {
    expect(advancedLaneOptions("text", {
      quiet: true,
      backend_mode: "llama_sdcpp",
      mystery_backend_key: "custom",
      model_param: "text.gguf",
      sdmodel: "image.safetensors"
    })).toEqual({
      quiet: true,
      backend_mode: "llama_sdcpp",
      mystery_backend_key: "custom",
      model_param: "text.gguf"
    });
  });

  it("keeps only voice section options for voice lanes", () => {
    expect(advancedLaneOptions("voice", {
      quiet: true,
      whispermodel: "whisper.gguf",
      musicdiffusion: "music-diffusion.gguf",
      sdmodel: "image.safetensors"
    })).toEqual({
      quiet: true,
      whispermodel: "whisper.gguf"
    });
  });

  it("keeps only music section options for music lanes", () => {
    expect(advancedLaneOptions("music", {
      quiet: true,
      whispermodel: "whisper.gguf",
      musicdiffusion: "music-diffusion.gguf",
      model_param: "text.gguf"
    })).toEqual({
      quiet: true,
      musicdiffusion: "music-diffusion.gguf"
    });
  });
});
