import { elements } from "./elements";
import {
  groupComponentsByNode,
  nodeByID,
  optionDefinition,
  selectedOptionsForNode,
  selectedThreadFieldsForNode
} from "./data";
import { state } from "./state";
import { advancedLaneOptions } from "./constructor-options";
import { backendModeKey, backendModes, type BackendMode } from "./constants";
import { gpuOptionKey, hasKind, issue, truthy } from "./utils";
import type { CookComponent, CookRequest, JsonValue, LaneKind, Options, PaletteComponentPayload, ValidationIssue } from "./types";

export function advancedCookRequest(): CookRequest {
  const selectedLanes = Object.entries(state.constructor.lanes)
    .filter((entry): entry is [LaneKind, PaletteComponentPayload] => entry[1] !== null);
  const components = selectedLanes.map(([lane, selected]) => componentForAdvancedLane(lane, selected));
  const options: Options = {};
  for (const [lane, selected] of selectedLanes) {
    Object.assign(options, advancedLaneOptions(lane, selected.model?.options ?? {}));
    Object.assign(options, state.constructor.laneOptions[lane] ?? {});
  }
  Object.assign(options, state.constructor.options);
  if (state.constructor.backendTouched) {
    options[backendModeKey] = state.constructor.backendMode;
  }
  return {
    id: elements.advancedCookIdInput.value.trim(),
    overwrite: elements.overwriteInput.checked,
    components,
    options
  };
}

export function localValidation(): ValidationIssue[] {
  const issues: ValidationIssue[] = [];
  const request = advancedCookRequest();
  if (!request.id) {
    issues.push(issue("warning", "id_missing", "Config id is empty.", "id"));
  }
  if (request.components.length === 0) {
    issues.push(issue("warning", "empty_constructor", "No lanes selected.", ""));
  }
  for (const [nodeID, components] of groupComponentsByNode(request.components)) {
    const node = nodeByID(nodeID);
    const selected = selectedOptionsForNode(nodeID, components, request.options ?? {});
    const backend = backendModeForOptions(selected, node?.backend_mode || "kobold");
    if (backend === "kobold" && hasKind(components, "image") && hasKind(components, "embeddings")) {
      issues.push(issue("error", "kobold_image_embeddings_mix", "Kobold cannot cook image and embeddings into the same config.", nodeID));
    }
    const maxThreads = node?.hardware?.max_threads || 0;
    for (const field of selectedThreadFieldsForNode(nodeID, components, request.options ?? {})) {
      if (maxThreads > 0 && field.value > maxThreads) {
        issues.push(issue("error", "thread_budget_exceeded", `${field.key} uses ${field.value} threads on a node with ${maxThreads} logical CPUs.`, field.key));
      }
    }
    if (node?.hardware?.gpu_backend === "rocm") {
      for (const [key, value] of Object.entries(selected)) {
        if (optionDefinition(key)?.cuda_only && truthy(value)) {
          issues.push(issue("error", "cuda_on_rocm", `${key} is CUDA-only on a ROCm node.`, key));
        }
      }
    }
    if (!node?.hardware?.gpu_backend || node.hardware.gpu_backend === "unknown") {
      for (const [key, value] of Object.entries(selected)) {
        if (gpuOptionKey(key) && truthy(value)) {
          issues.push(issue("warning", "gpu_backend_unknown", "GPU backend could not be inferred.", nodeID));
          break;
        }
      }
    }
    for (const [key] of Object.entries(selected)) {
      const definition = optionDefinition(key);
      if (!definition?.known) {
        continue;
      }
      if ((definition.backends?.length ?? 0) > 0 && !(definition.backends ?? []).includes(backend)) {
        issues.push(issue("warning", "unsupported_option", `${key} is not marked as supported by ${backend}.`, key));
      }
    }
  }
  return issues;
}

function backendModeForOptions(options: Options, fallback: string): string {
  const value = options[backendModeKey];
  if (typeof value === "string" && backendModes.includes(value as BackendMode)) {
    return value;
  }
  return backendModes.includes(fallback as BackendMode) ? fallback : "kobold";
}

export function componentForAdvancedLane(lane: LaneKind, selected: PaletteComponentPayload): CookComponent {
  const targetNodeID = state.constructor.targetNodes[lane] || selected.component.node_id || "";
  const targetNode = nodeByID(targetNodeID);
  const component: CookComponent = {
    ...selected.component,
    node_id: targetNodeID,
    node_url: targetNode?.node_url || selected.component.node_url || ""
  };
  if (targetNodeID && selected.component.node_id && targetNodeID !== selected.component.node_id) {
    const file = modelFileForLane(lane, selected);
    if (file.path) {
      component.source = "file";
      component.file_path = file.path;
      if (file.optionKey) {
        component.option_key = file.optionKey;
      } else {
        delete component.option_key;
      }
      delete component.model_id;
      delete component.image_id;
    }
  }
  return component;
}

interface LaneModelFile {
  path: string;
  optionKey?: string;
}

function modelFileForLane(lane: LaneKind, selected: PaletteComponentPayload): LaneModelFile {
  const options = selected.model?.options ?? {};
  if (lane === "image") {
    return {path: stringOption(options.sdmodel) || selected.file?.path || ""};
  }
  if (lane === "embeddings") {
    return {path: stringOption(options.embeddingsmodel) || selected.file?.path || ""};
  }
  if (lane === "voice") {
    return firstModelFile(options, ["whispermodel", "ttsmodel", "ttswavtokenizer", "ttsdir"], selected.file?.path);
  }
  if (lane === "music") {
    return firstModelFile(options, ["musicllm", "musicembeddings", "musicdiffusion", "musicvae"], selected.file?.path);
  }
  return {path: stringOption(options.model_param) || firstStringOption(options.model) || selected.file?.path || ""};
}

function firstModelFile(options: Options, keys: string[], fallback: string | undefined): LaneModelFile {
  for (const key of keys) {
    const path = stringOption(options[key]);
    if (path) {
      return {path, optionKey: key};
    }
  }
  const fallbackPath = fallback || "";
  if (!fallbackPath) {
    return {path: ""};
  }
  const optionKey = keys[0];
  return optionKey ? {path: fallbackPath, optionKey} : {path: fallbackPath};
}

function stringOption(value: JsonValue | undefined): string {
  return typeof value === "string" ? value.trim() : "";
}

function firstStringOption(value: JsonValue | undefined): string {
  if (typeof value === "string") {
    return value.trim();
  }
  if (Array.isArray(value)) {
    for (const item of value) {
      if (typeof item === "string" && item.trim()) {
        return item.trim();
      }
    }
  }
  return "";
}
