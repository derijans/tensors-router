import type { ConstructorContext, ConstructorDraft, CookComponent, Options, PaletteComponentPayload } from "../../../types";
import { numberOption, threadCountOption } from "../../../domain/option-values";
import { inventoryOptionDefinition } from "../../../domain/inventory";

interface SelectedThreadField {
  key: string;
  value: number;
}

export function selectedThreadFieldsForNode(context: ConstructorContext, nodeID: string, components: CookComponent[], options: Options): SelectedThreadField[] {
  const fields: SelectedThreadField[] = [];
  for (const [key, value] of Object.entries(selectedOptionsForNode(context.draft, nodeID, components, options))) {
    if (!threadCountOption(key, inventoryOptionDefinition(context.inventory, key))) {
      continue;
    }
    const count = numberOption(value);
    if (count > 0) {
      fields.push({key, value: count});
    }
  }
  return fields.sort((left, right) => left.key.localeCompare(right.key));
}

export function selectedOptionsForNode(draft: ConstructorDraft, nodeID: string, components: CookComponent[], options: Options): Options {
  const selected: Options = {};
  for (const component of components) {
    const item = draft.lanes[component.kind];
    const targetNodeID = draft.targetNodes[component.kind] || item?.component.node_id || "";
    if (!item || (targetNodeID || "") !== (nodeID || "")) {
      continue;
    }
    Object.assign(selected, item.model?.options ?? {});
    Object.assign(selected, draft.laneOptions[component.kind] ?? {});
  }
  Object.assign(selected, options);
  return selected;
}

export function selectedOptionsForInspector(draft: ConstructorDraft): Options {
  const selected: Options = {};
  for (const item of selectedConstructorPayloads(draft)) {
    Object.assign(selected, item.model?.options ?? {});
    Object.assign(selected, draft.laneOptions[item.lane] ?? {});
  }
  Object.assign(selected, draft.options);
  return selected;
}

export function usedPaths(selected: PaletteComponentPayload): string[] {
  const options = selected.model?.options ?? {};
  const values: string[] = [];
  for (const key of [
    "model_param",
    "model",
    "sdmodel",
    "embeddingsmodel",
    "mmproj",
    "sdvae",
    "sdt5xxl",
    "sdclipl",
    "sdclipg",
    "sdupscaler",
    "whispermodel",
    "whispercpp_vad_model",
    "ttsmodel",
    "ttswavtokenizer",
    "ttsdir",
    "musicllm",
    "musicembeddings",
    "musicdiffusion",
    "musicvae"
  ]) {
    const value = options[key];
    if (typeof value === "string" && value.trim()) {
      values.push(`${key}: ${value}`);
    } else if (Array.isArray(value)) {
      for (const item of value) {
        if (typeof item === "string" && item.trim()) {
          values.push(`${key}: ${item}`);
        }
      }
    }
  }
  if (selected.file?.path) {
    values.push(`file: ${selected.file.path}`);
  }
  return values;
}

function selectedConstructorPayloads(draft: ConstructorDraft): PaletteComponentPayload[] {
  return Object.values(draft.lanes).filter((item): item is PaletteComponentPayload => item !== null);
}
