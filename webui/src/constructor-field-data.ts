import type { FieldPreset, JsonValue, LaneKind, Options, PaletteComponentPayload } from "./types";

export const rawFileOptionKeys: Record<Extract<LaneKind, "voice" | "music">, string[]> = {
  voice: ["whispermodel", "ttsmodel", "ttswavtokenizer", "ttsdir"],
  music: ["musicllm", "musicembeddings", "musicdiffusion", "musicvae"]
};

export function rawFileKeysForLane(lane: LaneKind): string[] {
  if (lane === "voice" || lane === "music") {
    return rawFileOptionKeys[lane];
  }
  return [];
}

export function requiresOptionAssignment(payload: PaletteComponentPayload, lane: LaneKind): boolean {
  return (lane === "voice" || lane === "music") &&
    payload.component.source === "file" &&
    !payload.component.option_key;
}

export function validAssignmentKey(lane: LaneKind, key: string): boolean {
  if (lane !== "voice" && lane !== "music") {
    return false;
  }
  return rawFileOptionKeys[lane].includes(key);
}

export function changedDraftValues(draft: Options, source: Options): Options {
  const values: Options = {};
  for (const [key, value] of Object.entries(draft)) {
    if (comparableJsonValue(value) !== comparableJsonValue(source[key])) {
      values[key] = value;
    }
  }
  return values;
}

export function cloneOptions(options: Options | undefined): Options {
  return JSON.parse(JSON.stringify(options || {})) as Options;
}

export function fieldPresetID(preset: FieldPreset): string {
  return `${preset.backendMode}\n${preset.section}\n${preset.name}`;
}

export function comparableJsonValue(value: JsonValue | undefined): string {
  return JSON.stringify(value ?? null) ?? "";
}
