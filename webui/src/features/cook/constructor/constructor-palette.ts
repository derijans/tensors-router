import { allNodeFiles, allNodeModels, allOptionDefinitions } from "../../../state/inventory-selectors";
import { isNonBlank } from "../../../lib/text";
import type { CookComponent, FileRecord, JsonValue, LaneKind, Model, PaletteEntry } from "../../../types";
import { formatBytes } from "../../../lib/format-bytes";
import { compareOptionKeys } from "../../../domain/option-keys";
import { laneAccent } from "../../../domain/lanes";
import { fileRoles } from "../../../domain/model-files";

export function configPaletteEntries(): PaletteEntry[] {
  return allNodeModels().filter(model => !model.disabled).flatMap(model => {
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
  return allOptionDefinitions().sort((left, right) => compareOptionKeys(left.key, right.key)).map(definition => ({
    title: definition.name || definition.key,
    subtitle: definition.key,
    badge: definition.lane || "option",
    accent: definition.known ? "info" : "warning",
    meta: [
      definition.value_type || "json",
      ...(definition.backends ?? []),
      definition.native_flag ?? "",
      definition.known ? "known" : "observed"
    ].filter(isNonBlank),
    payload: {type: "option", key: definition.key}
  }));
}

function modelEntry(kind: LaneKind, model: Model): PaletteEntry {
  const id = kind === "image" ? model.public_image_id || model.image_id || model.local_id : model.public_id || model.local_id;
  return {
    title: id,
    subtitle: model.filename || "",
    badge: kind,
    accent: laneAccent(kind),
    meta: [model.node_id || "", model.backend_mode || "", optionCount(model.options)].filter(isNonBlank),
    payload: {
      type: "component",
      lane: kind,
      label: id,
      subtitle: model.filename || "",
      meta: [model.node_id || "", model.backend_mode || ""].filter(isNonBlank),
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
    accent: laneAccent(kind),
    meta: [file.node_id || "", formatBytes(file.size || 0)].filter(isNonBlank),
    payload: {
      type: "component",
      lane: kind,
      label: file.basename,
      subtitle: file.path,
      meta: [file.node_id || "", "file"].filter(isNonBlank),
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
