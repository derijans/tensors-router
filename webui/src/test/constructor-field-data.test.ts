import { describe, expect, it } from "vitest";
import {
  changedDraftValues,
  fieldPresetID,
  rawFileKeysForLane,
  requiresOptionAssignment,
  validAssignmentKey
} from "../constructor-field-data";
import { laneKinds, laneMetadata } from "../constants";
import type { PaletteComponentPayload } from "../types";

describe("constructor lane metadata", () => {
  it("defines the five cook lanes in primary board order", () => {
    expect(laneKinds).toEqual(["text", "image", "embeddings", "voice", "music"]);
    expect(laneMetadata.voice.section).toBe("voice");
    expect(laneMetadata.music.section).toBe("music");
  });
});

describe("constructor field staging data", () => {
  it("requires exact option assignment for raw voice and music files only", () => {
    const payload = filePayload("voice");
    expect(requiresOptionAssignment(payload, "voice")).toBe(true);
    expect(requiresOptionAssignment({...payload, component: {...payload.component, option_key: "ttsmodel"}}, "voice")).toBe(false);
    expect(requiresOptionAssignment(filePayload("text"), "text")).toBe(false);
  });

  it("exposes exact raw file assignment keys", () => {
    expect(rawFileKeysForLane("voice")).toEqual(["whispermodel", "ttsmodel", "ttswavtokenizer", "ttsdir"]);
    expect(rawFileKeysForLane("music")).toEqual(["musicllm", "musicembeddings", "musicdiffusion", "musicvae"]);
    expect(validAssignmentKey("music", "musicvae")).toBe(true);
    expect(validAssignmentKey("music", "sdmodel")).toBe(false);
  });

  it("stores only staged values that differ from the source", () => {
    expect(changedDraftValues({
      whispermodel: "same.gguf",
      ttsmodel: "override.gguf"
    }, {
      whispermodel: "same.gguf",
      ttsmodel: "source.gguf"
    })).toEqual({
      ttsmodel: "override.gguf"
    });
  });

  it("scopes preset ids by backend and section", () => {
    expect(fieldPresetID({
      name: "stage",
      backendMode: "kobold",
      section: "voice",
      values: {ttsgpu: true}
    })).toBe("kobold\nvoice\nstage");
  });
});

function filePayload(lane: "text" | "voice"): PaletteComponentPayload {
  return {
    type: "component",
    lane,
    label: `${lane}.gguf`,
    subtitle: `C:/models/${lane}.gguf`,
    meta: [],
    component: {
      kind: lane,
      node_id: "node-a",
      source: "file",
      file_path: `C:/models/${lane}.gguf`
    }
  };
}
