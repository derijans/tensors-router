import type { FieldPreset, JsonValue, LaneKind, OptionDefinition, Options, PaletteComponentPayload } from "../../../types";
import { laneMetadata } from "../../../domain/lanes";
import { compareOptionKeys, jinjaKwargsPrecedenceKey } from "../../../domain/option-keys";
import { cloneOptions, optionSection, optionValueText } from "../../../domain/option-values";

export const rawFileOptionKeys: Record<Extract<LaneKind, "voice" | "music">, string[]> = {
  voice: ["whispermodel", "whispercpp_vad_model", "ttsmodel", "ttswavtokenizer", "ttsdir"],
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

export function fieldPresetID(preset: FieldPreset): string {
  return `${preset.backendMode}\n${preset.section}\n${preset.name}`;
}

export function comparableJsonValue(value: JsonValue | undefined): string {
  return JSON.stringify(value ?? null) ?? "";
}

export interface FieldDiff {
  key: string;
  label: string;
  sourceText: string;
  hasOverride: boolean;
  changed: boolean;
  shownValue: JsonValue | undefined;
  draftText: string;
}

export function fieldDiff(key: string, label: string, sourceValue: JsonValue | undefined, draft: Options): FieldDiff {
  const effectiveSourceValue = key === jinjaKwargsPrecedenceKey && (sourceValue === undefined || sourceValue === null) ? "config" : sourceValue;
  const hasOverride = Object.hasOwn(draft, key);
  const draftValue = hasOverride ? draft[key] : undefined;
  return {
    key,
    label,
    sourceText: optionValueText(effectiveSourceValue),
    hasOverride,
    changed: hasOverride && comparableJsonValue(draftValue) !== comparableJsonValue(effectiveSourceValue),
    shownValue: hasOverride ? draftValue : effectiveSourceValue,
    draftText: hasOverride ? optionValueText(draftValue) : ""
  };
}

export function editorFieldKeys(lane: LaneKind, source: Options, draft: Options, definitions: readonly OptionDefinition[], definitionForKey: (key: string) => OptionDefinition | undefined): string[] {
  const section = laneMetadata[lane].section;
  const keys = new Set(definitions.filter(definition => optionSection(definition) === section).map(definition => definition.key));
  for (const key of [...Object.keys(source), ...Object.keys(draft), ...rawFileKeysForLane(lane)]) {
    const definition = definitionForKey(key);
    if (!definition || optionSection(definition) === section) {
      keys.add(key);
    }
  }
  return Array.from(keys).sort(compareOptionKeys);
}

export function addableSectionKeys(lane: LaneKind, usedKeys: readonly string[], definitions: readonly OptionDefinition[]): string[] {
  const used = new Set(usedKeys);
  const section = laneMetadata[lane].section;
  return definitions
    .filter(definition => optionSection(definition) === section && !used.has(definition.key))
    .map(definition => definition.key)
    .sort(compareOptionKeys);
}

export function sourceOptionsForPayload(payload: PaletteComponentPayload | null | undefined): Options {
  return cloneOptions(payload?.model?.options ?? {});
}

export function assignedPayload(payload: PaletteComponentPayload, optionKey: string): PaletteComponentPayload {
  return {...payload, component: {...payload.component, option_key: optionKey}};
}
