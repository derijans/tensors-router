import { state } from "./state.js";
import { fileRoles, formatBytes, kindColor, numberOption } from "./utils.js";

export function allNodeModels() {
  return (state.inventory?.nodes || []).flatMap(node => node.models || []);
}

export function allNodeFiles() {
  return (state.inventory?.nodes || []).flatMap(node => node.files || []);
}

export function allOptionDefinitions() {
  return [
    ...(state.inventory?.option_catalog || []),
    ...(state.inventory?.observed_options || [])
  ];
}

export function optionDefinition(key) {
  return allOptionDefinitions().find(option => option.key === key);
}

export function nodeByID(nodeID) {
  return (state.inventory?.nodes || []).find(node => node.node_id === nodeID);
}

export function filteredModels(query) {
  const models = state.inventory?.models?.length ? state.inventory.models : allNodeModels();
  if (!query) {
    return models;
  }
  return models.filter(model => JSON.stringify(model).toLowerCase().includes(query));
}

export function filteredFiles(query) {
  const files = allNodeFiles();
  if (!query) {
    return files;
  }
  return files.filter(file => JSON.stringify(file).toLowerCase().includes(query));
}

export function groupComponentsByNode(components) {
  const groups = new Map();
  for (const component of components) {
    const nodeID = component.node_id || state.inventory?.node_id || "local";
    if (!groups.has(nodeID)) {
      groups.set(nodeID, []);
    }
    groups.get(nodeID).push(component);
  }
  return groups;
}

export function selectedThreadFieldsForNode(nodeID, components, options) {
  const fields = [];
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

export function selectedOptionsForNode(nodeID, components, options) {
  const selected = {};
  for (const component of components) {
    const item = state.constructor.lanes[component.kind];
    const targetNodeID = state.constructor.targetNodes[component.kind] || item?.component?.node_id || "";
    if (!item || (targetNodeID || "") !== (nodeID || "")) {
      continue;
    }
    Object.assign(selected, item.model?.options || {});
  }
  Object.assign(selected, options);
  return selected;
}

export function selectedOptionsForInspector() {
  const selected = {};
  for (const item of Object.values(state.constructor.lanes).filter(Boolean)) {
    Object.assign(selected, item.model?.options || {});
  }
  Object.assign(selected, state.constructor.options);
  return selected;
}

export function usedPaths(selected) {
  const options = selected.model?.options || {};
  const values = [];
  for (const key of ["model_param", "model", "sdmodel", "embeddingsmodel", "mmproj", "sdvae", "sdt5xxl", "sdclipl", "sdclipg", "sdupscaler"]) {
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

export function componentOption(kind, model) {
  return optionValue(JSON.stringify({
    kind,
    node_id: model.node_id,
    node_url: model.node_url || "",
    source: "config",
    model_id: model.local_id,
    image_id: kind === "image" ? model.image_id : ""
  }), `${model.node_id} / ${kind === "image" ? model.public_image_id || model.image_id : model.public_id || model.local_id}`);
}

export function fileOption(kind, file) {
  return optionValue(JSON.stringify({
    kind,
    node_id: file.node_id,
    source: "file",
    file_path: file.path
  }), `${file.node_id} / ${file.basename}`);
}

export function configPaletteEntries() {
  return allNodeModels().flatMap(model => {
    const entries = [];
    if (model.has_llm) entries.push(modelEntry("text", model));
    if (model.has_image) entries.push(modelEntry("image", model));
    if (model.has_embeddings) entries.push(modelEntry("embeddings", model));
    return entries;
  });
}

export function filePaletteEntries() {
  return allNodeFiles().flatMap(file => {
    const entries = [];
    if (fileRoles(file).includes("llm")) entries.push(fileEntry("text", file));
    if (fileRoles(file).includes("image")) entries.push(fileEntry("image", file));
    if (fileRoles(file).includes("embeddings")) entries.push(fileEntry("embeddings", file));
    return entries;
  });
}

export function optionPaletteEntries() {
  return allOptionDefinitions().map(definition => ({
    title: definition.name || definition.key,
    subtitle: definition.key,
    badge: definition.lane || "option",
    color: definition.known ? "cyan" : "amber",
    meta: [
      definition.value_type || "json",
      ...(definition.backends || []),
      definition.native_flag || "",
      definition.known ? "known" : "observed"
    ].filter(Boolean),
    payload: {type: "option", key: definition.key}
  }));
}

function threadCountOption(key) {
  const definition = optionDefinition(key);
  if (definition) {
    return definition.value_type === "number" && definition.key.endsWith("threads");
  }
  return String(key || "").trim().toLowerCase().endsWith("threads");
}

function modelEntry(kind, model) {
  const id = kind === "image" ? model.public_image_id || model.image_id : model.public_id || model.local_id;
  return {
    title: id,
    subtitle: model.filename || "",
    badge: kind,
    color: kindColor(kind),
    meta: [model.node_id || "", model.backend_mode || "", optionCount(model.options)].filter(Boolean),
    payload: {
      type: "component",
      lane: kind,
      label: id,
      subtitle: model.filename || "",
      meta: [model.node_id || "", model.backend_mode || ""].filter(Boolean),
      component: JSON.parse(componentOption(kind, model).value),
      model
    }
  };
}

function fileEntry(kind, file) {
  return {
    title: file.basename,
    subtitle: file.path,
    badge: kind,
    color: kindColor(kind),
    meta: [file.node_id || "", formatBytes(file.size || 0)].filter(Boolean),
    payload: {
      type: "component",
      lane: kind,
      label: file.basename,
      subtitle: file.path,
      meta: [file.node_id || "", "file"].filter(Boolean),
      component: JSON.parse(fileOption(kind, file).value),
      file
    }
  };
}

function optionValue(value, label) {
  return {value, label};
}

function optionCount(options) {
  const count = Object.keys(options || {}).length;
  return count ? `${count} options` : "";
}
