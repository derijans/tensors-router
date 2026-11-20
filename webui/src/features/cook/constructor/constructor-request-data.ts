import { componentsByNode, inventoryNode, inventoryOptionDefinition } from "../../../domain/inventory";
import { advancedLaneOptions } from "./constructor-lane-options-data";
import type { ConstructorContext, CookComponent, CookRequest, JsonValue, LaneKind, Options, PaletteComponentPayload, ValidationIssue } from "../../../types";
import { backendModeKey, backendModeOrDefault, isBackendMode } from "../../../domain/backend-mode";
import { issue } from "../../../domain/validation";
import { hasKind } from "../../../domain/cook-components";
import { gpuOptionKey } from "../../../domain/gpu-options";
import { truthy } from "../../../domain/option-values";
import { selectedOptionsForNode, selectedThreadFieldsForNode } from "./constructor-selection-data";

export interface CookTarget {
  id: string;
  overwrite: boolean;
}

export function advancedCookRequest(context: ConstructorContext, target: CookTarget): CookRequest {
  const {draft, inventory} = context;
  const selectedLanes = Object.entries(draft.lanes)
    .filter((entry): entry is [LaneKind, PaletteComponentPayload] => entry[1] !== null);
  const components = selectedLanes.map(([lane, selected]) => componentForAdvancedLane(context, lane, selected));
  const options: Options = {};
  for (const [lane, selected] of selectedLanes) {
    Object.assign(options, advancedLaneOptions(inventory, lane, selected.model?.options ?? {}));
    Object.assign(options, draft.laneOptions[lane] ?? {});
  }
  Object.assign(options, draft.options);
  if (draft.backendTouched) {
    options[backendModeKey] = draft.backendMode;
  }
  return {
    id: target.id.trim(),
    overwrite: target.overwrite,
    components,
    options
  };
}

export function localValidationIssues(context: ConstructorContext, request: CookRequest): ValidationIssue[] {
  const issues: ValidationIssue[] = [];
  if (!request.id) {
    issues.push(issue("warning", "id_missing", "Config id is empty.", "id"));
  }
  if (request.components.length === 0) {
    issues.push(issue("warning", "empty_constructor", "No lanes selected.", ""));
  }
  for (const [nodeID, components] of componentsByNode(context.inventory, request.components)) {
    issues.push(...nodeValidationIssues(context, nodeID, components, request.options ?? {}));
  }
  return issues;
}

function nodeValidationIssues(context: ConstructorContext, nodeID: string, components: CookComponent[], requestOptions: Options): ValidationIssue[] {
  const node = inventoryNode(context.inventory, nodeID);
  const selected = selectedOptionsForNode(context.draft, nodeID, components, requestOptions);
  const backend = backendModeForOptions(selected, node?.backend_mode || "kobold");
  const gpuBackend = node?.hardware?.gpu_backend;
  return [
    ...mixedLaneIssues(nodeID, components, backend),
    ...threadBudgetIssues(context, nodeID, components, requestOptions, node?.hardware?.max_threads || 0),
    ...cudaOnROCmIssues(context, selected, gpuBackend),
    ...unknownGPUBackendIssues(nodeID, selected, gpuBackend),
    ...unsupportedOptionIssues(context, selected, backend)
  ];
}

function mixedLaneIssues(nodeID: string, components: CookComponent[], backend: string): ValidationIssue[] {
  if (backend !== "kobold" || !hasKind(components, "image") || !hasKind(components, "embeddings")) {
    return [];
  }
  return [issue("error", "kobold_image_embeddings_mix", "Kobold cannot cook image and embeddings into the same config.", nodeID)];
}

function threadBudgetIssues(context: ConstructorContext, nodeID: string, components: CookComponent[], requestOptions: Options, maxThreads: number): ValidationIssue[] {
  if (maxThreads <= 0) {
    return [];
  }
  return selectedThreadFieldsForNode(context, nodeID, components, requestOptions)
    .filter(field => field.value > maxThreads)
    .map(field => issue("error", "thread_budget_exceeded", `${field.key} uses ${field.value} threads on a node with ${maxThreads} logical CPUs.`, field.key));
}

function cudaOnROCmIssues(context: ConstructorContext, selected: Options, gpuBackend: string | undefined): ValidationIssue[] {
  if (gpuBackend !== "rocm") {
    return [];
  }
  return Object.entries(selected)
    .filter(([key, value]) => inventoryOptionDefinition(context.inventory, key)?.cuda_only && truthy(value))
    .map(([key]) => issue("error", "cuda_on_rocm", `${key} is CUDA-only on a ROCm node.`, key));
}

function unknownGPUBackendIssues(nodeID: string, selected: Options, gpuBackend: string | undefined): ValidationIssue[] {
  if (gpuBackend && gpuBackend !== "unknown") {
    return [];
  }
  const usesGPU = Object.entries(selected).some(([key, value]) => gpuOptionKey(key) && truthy(value));
  return usesGPU ? [issue("warning", "gpu_backend_unknown", "GPU backend could not be inferred.", nodeID)] : [];
}

function unsupportedOptionIssues(context: ConstructorContext, selected: Options, backend: string): ValidationIssue[] {
  return Object.keys(selected)
    .filter(key => optionSupportExcludesBackend(context, key, backend))
    .map(key => issue("warning", "unsupported_option", `${key} is not marked as supported by ${backend}.`, key));
}

function optionSupportExcludesBackend(context: ConstructorContext, key: string, backend: string): boolean {
  const definition = inventoryOptionDefinition(context.inventory, key);
  const backends = definition?.backends ?? [];
  return Boolean(definition?.known) && backends.length > 0 && !backends.includes(backend);
}

function backendModeForOptions(options: Options, fallback: string): string {
  const value = options[backendModeKey];
  return isBackendMode(value) ? value : backendModeOrDefault(fallback);
}

function componentForAdvancedLane(context: ConstructorContext, lane: LaneKind, selected: PaletteComponentPayload): CookComponent {
  const targetNodeID = context.draft.targetNodes[lane] || selected.component.node_id || "";
  const targetNode = inventoryNode(context.inventory, targetNodeID);
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
    return firstModelFile(options, ["whispermodel", "whispercpp_vad_model", "ttsmodel", "ttswavtokenizer", "ttsdir"], selected.file?.path);
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
