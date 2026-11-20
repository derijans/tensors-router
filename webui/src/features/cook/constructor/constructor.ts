import { requiresOptionAssignment } from "./constructor-field-data";
import { emptyLaneOptions, emptyLanes, emptyLaneTargets, state } from "../../../state/app-state";
import { openFieldEditor } from "./constructor-field-editor";
import { renderConstructor, renderInspector, renderValidation } from "./constructor-render";
import { optionDefinition } from "../../../state/inventory-selectors";
import { clearConstructorConversions, clearConversionScope, discardConversion, invalidateAcceptedConversions, recordConversion } from "../conversions";
import type { PalettePayload } from "../../../types";
import { isLaneKind } from "../../../domain/lanes";
import { isBackendMode } from "../../../domain/backend-mode";
import { jinjaKwargsKey, jinjaKwargsPrecedenceKey } from "../../../domain/option-keys";
import { emptyValueForType, parseOptionInput } from "../../../domain/option-values";

export function addPayload(payload: PalettePayload | undefined, lane?: string): void {
  if (!payload) {
    return;
  }
  if (payload.type === "option") {
    addOption(payload.key);
    return;
  }
  const targetLane = isLaneKind(lane) ? lane : payload.lane;
  if (targetLane !== payload.lane) {
    return;
  }
  if (requiresOptionAssignment(payload, targetLane)) {
    openFieldEditor(targetLane, payload);
    return;
  }
  state.constructor.lanes[targetLane] = payload;
  invalidateAcceptedConversions();
  renderConstructor();
}

export function addOption(key: string): void {
  const definition = optionDefinition(key);
  if (!definition) {
    return;
  }
  if (!Object.hasOwn(state.constructor.options, key)) {
    state.constructor.options[key] = emptyValueForType(definition);
  }
  if (key === jinjaKwargsKey && !Object.hasOwn(state.constructor.options, jinjaKwargsPrecedenceKey)) {
    state.constructor.options[jinjaKwargsPrecedenceKey] = "config";
  }
  invalidateAcceptedConversions();
  renderConstructor();
}

export function clearConstructor(): void {
  state.constructor.lanes = emptyLanes();
  state.constructor.targetNodes = emptyLaneTargets();
  state.constructor.laneOptions = emptyLaneOptions();
  state.constructor.backendMode = "kobold";
  state.constructor.backendTouched = false;
  state.constructor.options = {};
  state.constructor.fieldEditor = null;
  clearConstructorConversions();
  invalidateAcceptedConversions();
  renderConstructor();
}

export function clearLane(lane: string): void {
  if (!isLaneKind(lane)) {
    return;
  }
  state.constructor.lanes[lane] = null;
  state.constructor.laneOptions[lane] = {};
  clearConversionScope(`lane-${lane}`);
  invalidateAcceptedConversions();
  renderConstructor();
}

export function editLaneFields(lane: string): void {
  if (!isLaneKind(lane) || !state.constructor.lanes[lane]) {
    return;
  }
  openFieldEditor(lane);
}

export function updateOptionInput(target: EventTarget | null): void {
  if (!(target instanceof HTMLInputElement) && !(target instanceof HTMLSelectElement)) {
    return;
  }
  const key = target.dataset.optionInput;
  if (!key) {
    return;
  }
  const parsed = parseOptionInput(optionDefinition(key), target.value);
  state.constructor.options[key] = parsed.value;
  recordConversion("constructor", key, parsed);
  target.setCustomValidity("");
  renderValidation();
}

export function removeOption(key: string): void {
  delete state.constructor.options[key];
  if (key === jinjaKwargsKey) {
    delete state.constructor.options[jinjaKwargsPrecedenceKey];
  }
  discardConversion("constructor", key);
  invalidateAcceptedConversions();
  renderConstructor();
}

export function toggleInspectorList(target: string): void {
  if (target === "used") {
    state.constructor.showUsedAll = !state.constructor.showUsedAll;
  }
  if (target === "options") {
    state.constructor.showOptionsAll = !state.constructor.showOptionsAll;
  }
  renderInspector();
}

export function updateLaneTarget(target: EventTarget | null): void {
  if (!(target instanceof HTMLSelectElement)) {
    return;
  }
  const lane = target.dataset.laneTarget;
  if (!isLaneKind(lane)) {
    return;
  }
  state.constructor.targetNodes[lane] = target.value;
  invalidateAcceptedConversions();
  renderConstructor();
}

export function updateConstructorBackendMode(value: string): void {
  if (!isBackendMode(value)) {
    return;
  }
  state.constructor.backendMode = value;
  state.constructor.backendTouched = true;
  invalidateAcceptedConversions();
  renderConstructor();
}
