import { laneMetadata } from "../../../domain/lanes";
import { isJsonObject } from "../../../lib/json";
import { state } from "../../../state/app-state";
import type { FieldPreset, LaneKind } from "../../../types";
import { fieldPresetID } from "./constructor-field-data";

const presetStorageKey = "tensors-router.constructorFieldPresets";

export function loadFieldPresets(): void {
  if (state.constructor.fieldPresets.length > 0) {
    return;
  }
  try {
    const parsed = JSON.parse(window.localStorage.getItem(presetStorageKey) || "[]") as unknown;
    state.constructor.fieldPresets = Array.isArray(parsed) ? parsed.filter(isFieldPreset) : [];
  } catch {
    state.constructor.fieldPresets = [];
  }
}

export function storeFieldPreset(preset: FieldPreset): void {
  state.constructor.fieldPresets = [
    ...state.constructor.fieldPresets.filter(item => fieldPresetID(item) !== fieldPresetID(preset)),
    preset
  ];
  window.localStorage.setItem(presetStorageKey, JSON.stringify(state.constructor.fieldPresets));
}

export function presetsForLane(lane: LaneKind, backendMode: string): FieldPreset[] {
  const section = laneMetadata[lane].section;
  return state.constructor.fieldPresets.filter(preset =>
    preset.section === section &&
    (!preset.backendMode || preset.backendMode === backendMode)
  );
}

function isFieldPreset(value: unknown): value is FieldPreset {
  if (!isJsonObject(value)) {
    return false;
  }
  return typeof value.name === "string" &&
    typeof value.backendMode === "string" &&
    typeof value.section === "string" &&
    isJsonObject(value.values);
}
