import { describe, expect, it } from "vitest";
import {
  changedTriggerSelection,
  doNotUnloadSelected,
  groupTriggersByKind,
  separateButtonLabel,
  selectedTriggerKeys,
  triggerKind,
  triggersFromSelection
} from "../separate-runtime-data";
import type { SeparateRuntimeCandidates, SeparateRuntimeSettings } from "../types";

const candidates: SeparateRuntimeCandidates = {
  lanes: ["text", "image", "embeddings", "voice", "music"],
  families: ["family:kobold", "family:llama_sdcpp", "family:vllm"],
  configs: ["config:qwen3-1.7b", "config:sdxl"]
};

function settings(overrides: Partial<SeparateRuntimeSettings> = {}): SeparateRuntimeSettings {
  return {run_separate: true, triggers: [], ...overrides};
}

describe("triggerKind", () => {
  it("classifies each trigger shape", () => {
    expect(triggerKind("none")).toBe("special");
    expect(triggerKind("all")).toBe("special");
    expect(triggerKind("text")).toBe("lane");
    expect(triggerKind("family:kobold")).toBe("family");
    expect(triggerKind("config:sdxl")).toBe("config");
  });
});

describe("groupTriggersByKind", () => {
  it("passes the candidate buckets through as fresh arrays", () => {
    const groups = groupTriggersByKind(candidates);
    expect(groups.configs).toEqual(["config:qwen3-1.7b", "config:sdxl"]);
    expect(groups.configs).not.toBe(candidates.configs);
  });
});

describe("selectedTriggerKeys", () => {
  it("drops the absorbing values so only real triggers stay ticked", () => {
    expect([...selectedTriggerKeys(settings({triggers: ["none"]}))]).toEqual([]);
    expect([...selectedTriggerKeys(settings({triggers: ["text", "family:kobold"]})).values()]).toEqual([
      "text",
      "family:kobold"
    ]);
  });
});

describe("doNotUnloadSelected", () => {
  it("is true for an empty set or an explicit none", () => {
    expect(doNotUnloadSelected(settings({triggers: []}))).toBe(true);
    expect(doNotUnloadSelected(settings({triggers: ["none"]}))).toBe(true);
    expect(doNotUnloadSelected(settings({triggers: ["text"]}))).toBe(false);
  });
});

describe("changedTriggerSelection", () => {
  it("makes none and all absorbing in both directions", () => {
    expect([...changedTriggerSelection(new Set(["text", "image"]), "all", true)]).toEqual(["all"]);
    expect([...changedTriggerSelection(new Set(["none"]), "text", true)]).toEqual(["text"]);
    expect([...changedTriggerSelection(new Set(["text"]), "text", false)]).toEqual([]);
  });
});

describe("triggersFromSelection", () => {
  it("normalizes to none, all, or a sorted list", () => {
    expect(triggersFromSelection(new Set())).toEqual(["none"]);
    expect(triggersFromSelection(new Set(["all", "text"]))).toEqual(["all"]);
    expect(triggersFromSelection(new Set(["voice", "image"]))).toEqual(["image", "voice"]);
  });
});

describe("separateButtonLabel", () => {
  it("counts only real triggers and never shows one when off", () => {
    expect(separateButtonLabel(false, settings())).toBe("Separate");
    expect(separateButtonLabel(true, settings({run_separate: false, triggers: ["text"]}))).toBe("Separate");
    expect(separateButtonLabel(true, settings({triggers: ["none"]}))).toBe("Separate · on");
    expect(separateButtonLabel(true, settings({triggers: ["text"]}))).toBe("Separate · on · 1 trigger");
    expect(separateButtonLabel(true, settings({triggers: ["text", "family:kobold"]}))).toBe("Separate · on · 2 triggers");
  });
});
