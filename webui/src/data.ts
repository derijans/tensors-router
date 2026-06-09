import { state } from "./state";
import { fileRoles, formatBytes, kindColor, numberOption } from "./utils";
import type {
  CookComponent,
  FileRecord,
  JsonValue,
  LaneKind,
  Model,
  OptionDefinition,
  Options,
  PaletteComponentPayload,
  PaletteEntry,
  SelectChoice
} from "./types";

interface SelectedThreadField {
  key: string;
  value: number;
}

export function allNodeModels(): Model[] {
  return (state.inventory?.nodes ?? []).flatMap(node => node.models ?? []);
}

export function allNodeFiles(): FileRecord[] {
  return (state.inventory?.nodes ?? []).flatMap(node => node.files ?? []);
}

export function allOptionDefinitions(): OptionDefinition[] {
  return [
    ...(state.inventory?.option_catalog ?? []),
    ...(state.inventory?.observed_options ?? [])
  ];
}

export function optionDefinition(key: string): OptionDefinition | undefined {
  return allOptionDefinitions().find(option => option.key === key);
}

export function nodeByID(nodeID: string) {
  return (state.inventory?.nodes ?? []).find(node => node.node_id === nodeID);
}

export function filteredModels(query: string): Model[] {
  const models = state.inventory?.models?.length ? state.inventory.models : allNodeModels();
  if (!query) {
    return models;
  }
  return models.filter(model => JSON.stringify(model).toLowerCase().includes(query));
}

export function filteredFiles(query: string): FileRecord[] {
  const files = allNodeFiles();
  if (!query) {
    return files;
  }
  return files.filter(file => JSON.stringify(file).toLowerCase().includes(query));
}

export function groupComponentsByNode(components: CookComponent[]): Map<string, CookComponent[]> {
  const groups = new Map<string, CookComponent[]>();
  for (const component of components) {
    const nodeID = component.node_id || state.inventory?.node_id || "local";
    const group = groups.get(nodeID) ?? [];
    group.push(component);
    groups.set(nodeID, group);
  }
  return groups;
}

export function selectedThreadFieldsForNode(nodeID: string, components: CookComponent[], options: Options): SelectedThreadField[] {
  const fields: SelectedThreadField[] = [];
  for (const [key, value] of Object.entries(selectedOptionsForNode(nodeID, components, options))) {
    if (!threadCountOption(key)) {
      continue;
    }
    const count = numberOption(value);
    if (count > 0) {
      fields.push({key, value: count});
    }
  }
  return fields.sort((left, right) => left.key.localeCompare(right.key));
}

export function selectedOptionsForNode(nodeID: string, components: CookComponent[], options: Options): Options {
  const selected: Options = {};
  for (const component of components) {
    const item = state.constructor.lanes[component.kind];
    const targetNodeID = state.constructor.targetNodes[component.kind] || item?.component.node_id || "";
    if (!item || (targetNodeID || "") !== (nodeID || "")) {
      continue;
    }
    Object.assign(selected, item.model?.options ?? {});
    Object.assign(selected, state.constructor.laneOptions[component.kind] ?? {});
  }
  Object.assign(selected, options);
  return selected;
}

export function selectedOptionsForInspector(): Options {
  const selected: Options = {};
  for (const item of selectedConstructorPayloads()) {
    Object.assign(selected, item.model?.options ?? {});
    Object.assign(selected, state.constructor.laneOptions[item.lane] ?? {});
  }
  Object.assign(selected, state.constructor.options);
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

export function configPaletteEntries(): PaletteEntry[] {
  return allNodeModels().flatMap(model => {
    const entries: PaletteEntry[] = [];
    if (model.has_llm) entries.push(modelEntry("text", model));
    if (model.has_image) entries.push(modelEntry("image", model));
    if (model.has_embeddings) entries.push(modelEntry("embeddings", model));
    if (model.has_voice) entries.push(modelEntry("voice", model));
    if (model.has_music) entries.push(modelEntry("music", model));
    return entries;
  });
}

export function filePaletteEntries(): PaletteEntry[] {
  return allNodeFiles().flatMap(file => {
    const entries: PaletteEntry[] = [];
    if (fileRoles(file).includes("llm")) entries.push(fileEntry("text", file));
    if (fileRoles(file).includes("image")) entries.push(fileEntry("image", file));
    if (fileRoles(file).includes("embeddings")) entries.push(fileEntry("embeddings", file));
    if (fileRoles(file).includes("voice")) entries.push(fileEntry("voice", file));
    if (fileRoles(file).includes("music")) entries.push(fileEntry("music", file));
    return entries;
  });
}

export function optionPaletteEntries(): PaletteEntry[] {
  return allOptionDefinitions().map(definition => ({
    title: definition.name || definition.key,
    subtitle: definition.key,
    badge: definition.lane || "option",
    color: definition.known ? "cyan" : "amber",
    meta: [
      definition.value_type || "json",
      ...(definition.backends ?? []),
      definition.native_flag ?? "",
      definition.known ? "known" : "observed"
    ].filter(nonEmptyString),
    payload: {type: "option", key: definition.key}
  }));
}

function threadCountOption(key: string): boolean {
  const definition = optionDefinition(key);
  if (definition) {
    return definition.value_type === "number" && definition.key.endsWith("threads");
  }
  return String(key || "").trim().toLowerCase().endsWith("threads");
}

function modelEntry(kind: LaneKind, model: Model): PaletteEntry {
  const id = kind === "image" ? model.public_image_id || model.image_id || model.local_id : model.public_id || model.local_id;
  return {
    title: id,
    subtitle: model.filename || "",
    badge: kind,
    color: kindColor(kind),
    meta: [model.node_id || "", model.backend_mode || "", optionCount(model.options)].filter(nonEmptyString),
    payload: {
      type: "component",
      lane: kind,
      label: id,
      subtitle: model.filename || "",
      meta: [model.node_id || "", model.backend_mode || ""].filter(nonEmptyString),
      component: componentForModel(kind, model),
      model
    }
  };
}

function fileEntry(kind: LaneKind, file: FileRecord): PaletteEntry {
  return {
    title: file.basename,
    subtitle: file.path,
    badge: kind,
    color: kindColor(kind),
    meta: [file.node_id || "", formatBytes(file.size || 0)].filter(nonEmptyString),
    payload: {
      type: "component",
      lane: kind,
      label: file.basename,
      subtitle: file.path,
      meta: [file.node_id || "", "file"].filter(nonEmptyString),
      component: componentForFile(kind, file),
      file
    }
  };
}

function componentForModel(kind: LaneKind, model: Model): CookComponent {
  const component: CookComponent = {
    kind,
    node_id: model.node_id,
    node_url: model.node_url || "",
    source: "config",
    model_id: model.local_id
  };
  if (kind === "image") {
    component.image_id = model.image_id || "";
  }
  return component;
}

function componentForFile(kind: LaneKind, file: FileRecord): CookComponent {
  return {
    kind,
    node_id: file.node_id,
    source: "file",
    file_path: file.path
  };
}

function optionCount(options: Record<string, JsonValue> | undefined): string {
  const count = Object.keys(options ?? {}).length;
  return count ? `${count} options` : "";
}

function selectedConstructorPayloads(): PaletteComponentPayload[] {
  return Object.values(state.constructor.lanes).filter((item): item is PaletteComponentPayload => item !== null);
}

function nonEmptyString(value: string): value is string {
  return value.trim() !== "";
}

export function optionValue(value: string, label: string): SelectChoice {
  return {value, label};
}
