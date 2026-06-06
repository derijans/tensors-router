import { allNodeFiles, nodeByID } from "./data.js";
import { state } from "./state.js";
import { fileRoles, optionValueLabel, parseOptionInput } from "./utils.js";

export const sectionOrder = ["llm", "image", "embed", "voice", "music", "runtime", "other"];

export const sectionLabels = {
  llm: "LLM",
  image: "Image",
  embed: "Embed",
  voice: "Voice",
  music: "Music",
  runtime: "Runtime",
  other: "Other"
};

export const sectionModelKeys = {
  llm: ["model_param", "model"],
  image: ["sdmodel"],
  embed: ["embeddingsmodel", "mmproj"],
  voice: ["ttsmodel", "whispermodel"],
  music: ["musicllm", "musicdiffusion"]
};

export function selectedNode() {
  return nodeByID(state.simpleCook.nodeID) || (state.inventory?.nodes || [])[0] || null;
}

export function selectedConfig() {
  const node = selectedNode();
  return (node?.models || []).find(model => model.local_id === state.simpleCook.configID) || null;
}

export function fieldRenderContext() {
  const node = selectedNode();
  const nodeID = node?.node_id || "";
  const nodeModels = node?.models || [];
  return {
    node,
    nodeFiles: allNodeFiles().filter(file => file.node_id === nodeID),
    nodeModels,
    otherNodeModels: nodeModels.filter(model => model.local_id !== state.simpleCook.configID),
    comparableBySection: new Map()
  };
}

export function groupedFieldKeys(fields, optionDefinition) {
  const groups = new Map(sectionOrder.map(section => [section, []]));
  for (const key of Object.keys(fields).sort((left, right) => left.localeCompare(right))) {
    const section = sectionForDefinition(optionDefinition(key));
    if (!groups.has(section)) {
      groups.set(section, []);
    }
    groups.get(section).push(key);
  }
  return sectionOrder
    .map(section => ({section, keys: groups.get(section) || []}))
    .filter(group => group.keys.length > 0);
}

export function fieldChoices(key, definition, context) {
  const choices = [
    ...(definition?.choices || []),
    ...modelChoicesForDefinition(definition, context),
    ...observedValuesForField(key, context)
  ];
  return uniqueValues(choices.map(choice => inputChoiceValue(choice, definition)));
}

export function comparisonClass(key, section, context) {
  const currentValue = state.simpleCook.fields?.[key];
  const currentModelValue = modelCohortValue(state.simpleCook.fields, section);
  const comparable = comparableModels(section, context);
  const values = comparable
    .map(model => model.options?.[key])
    .filter(value => !emptyComparableValue(value));
  if (values.length === 0) {
    if (currentModelValue && comparable.length === 0 && !emptyComparableValue(currentValue)) {
      return "compare-same";
    }
    return "compare-none";
  }
  const current = comparableValue(currentValue);
  if (values.every(value => comparableValue(value) === current)) {
    return "compare-same";
  }
  return "compare-different";
}

export function sidebarValueRows(key, type, optionDefinition, context) {
  const section = sectionForDefinition(optionDefinition(key));
  const models = type === "model" ? comparableModels(section, context) : context.otherNodeModels;
  const rows = [];
  const seen = new Set();
  for (const model of models) {
    const value = model.options?.[key];
    if (emptyComparableValue(value)) {
      continue;
    }
    const label = optionValueLabel(value);
    const seenKey = `${label}\n${model.local_id}`;
    if (seen.has(seenKey)) {
      continue;
    }
    seen.add(seenKey);
    rows.push({value: label, config: configLabel(model)});
  }
  return rows;
}

export function defaultConfigForNode(node) {
  const hardware = node?.hardware || {};
  const values = {
    quiet: true,
    nomodel: false,
    contextsize: 4096,
    threads: hardware.max_threads ? Math.max(1, Math.floor(hardware.max_threads / 2)) : -1,
    batchsize: 512,
    usemmap: true,
    usemlock: false,
    gpulayers: hardware.gpu_backend && hardware.gpu_backend !== "cpu" && hardware.gpu_backend !== "unknown" ? "auto" : "0"
  };
  if (hardware.gpu_backend === "cuda") {
    values.usecuda = true;
  }
  if (hardware.gpu_backend === "vulkan") {
    values.usevulkan = true;
  }
  const nodeURL = parseURL(node?.node_url || "");
  if (nodeURL) {
    values.host = nodeURL.hostname;
    if (nodeURL.port) {
      values.port = Number(nodeURL.port);
    }
  }
  return values;
}

export function defaultFieldValue(definition) {
  if (definition?.default !== undefined && definition.default !== "") {
    return parseOptionInput(definition, definition.default);
  }
  switch (definition?.value_type) {
    case "bool":
      return false;
    case "number":
      return 0;
    case "json":
      return {};
    default:
      return "";
  }
}

export function cloneValue(value) {
  return JSON.parse(JSON.stringify(value || {}));
}

export function optionValue(value, label) {
  return {value, label};
}

export function nodeLabel(node) {
  return `${node.node_id || "node"} / ${node.backend_mode || "backend"}`;
}

export function configLabel(model) {
  return `${model.local_id || model.public_id || "config"} / ${model.filename || ""}`;
}

export function suggestedConfigID(node, fallback) {
  const nodePrefix = (node?.node_id || "node").toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^-|-$/g, "");
  return `${nodePrefix || "node"}-${fallback}`;
}

export function safeID(value) {
  return String(value).replace(/[^a-z0-9_-]/gi, "-");
}

function modelChoicesForDefinition(definition, context) {
  if (!definition?.model_role) {
    return [];
  }
  const files = context.nodeFiles
    .filter(file => roleMatchesDefinition(fileRoles(file), definition.model_role))
    .map(file => file.path);
  const models = context.nodeModels.flatMap(model => modelPathsForRole(model, definition.model_role));
  return [...files, ...models];
}

function observedValuesForField(key, context) {
  return context.nodeModels
    .map(model => model.options?.[key])
    .filter(value => !emptyComparableValue(value))
    .map(optionValueLabel);
}

function comparableModels(section, context) {
  if (context.comparableBySection.has(section)) {
    return context.comparableBySection.get(section);
  }
  const currentModelValue = modelCohortValue(state.simpleCook.fields, section);
  let models = context.otherNodeModels;
  if (!currentModelValue) {
    context.comparableBySection.set(section, models);
    return models;
  }
  models = models.filter(model => modelCohortValue(model.options || {}, section) === currentModelValue);
  context.comparableBySection.set(section, models);
  return models;
}

function sectionForDefinition(definition) {
  return definition?.section || "other";
}

function modelCohortValue(options, section) {
  for (const key of sectionModelKeys[section] || []) {
    const value = options?.[key];
    if (!emptyComparableValue(value)) {
      return comparableValue(value);
    }
  }
  return "";
}

function roleMatchesDefinition(roles, role) {
  if (role === "llm") {
    return roles.includes("llm");
  }
  if (role === "image") {
    return roles.includes("image");
  }
  if (role === "embeddings") {
    return roles.includes("embeddings") || roles.includes("llm");
  }
  if (role === "multimodal") {
    return roles.includes("multimodal");
  }
  if (role === "vae") {
    return roles.includes("vae");
  }
  if (role === "clip") {
    return roles.includes("clip");
  }
  if (role === "t5") {
    return roles.includes("t5");
  }
  if (role === "upscaler") {
    return roles.includes("upscaler");
  }
  if (role === "lora") {
    return roles.includes("lora");
  }
  return true;
}

function modelPathsForRole(model, role) {
  const capabilities = model.capabilities || {};
  const values = [];
  if (role === "llm" && typeof model.filename === "string") {
    values.push(model.filename);
  }
  if (role === "image" && capabilities.image?.model) {
    values.push(capabilities.image.model);
  }
  if (role === "embeddings" && capabilities.embeddings?.model) {
    values.push(capabilities.embeddings.model);
  }
  if (role === "multimodal" && capabilities.multimodal?.projector) {
    values.push(capabilities.multimodal.projector);
  }
  if (role === "vae" && capabilities.image?.vae) {
    values.push(capabilities.image.vae);
  }
  if (role === "clip") {
    values.push(capabilities.image?.clip1, capabilities.image?.clip2, capabilities.image?.clip_l, capabilities.image?.clip_g);
  }
  if (role === "t5" && capabilities.image?.t5xxl) {
    values.push(capabilities.image.t5xxl);
  }
  if (role === "upscaler" && capabilities.image?.upscaler) {
    values.push(capabilities.image.upscaler);
  }
  if (role === "lora") {
    values.push(...(capabilities.image?.lora || []));
  }
  return values.filter(Boolean);
}

function inputChoiceValue(value, definition) {
  if (definition?.value_type === "json") {
    try {
      JSON.parse(value);
      return value;
    } catch {
      return JSON.stringify(value);
    }
  }
  return value;
}

function emptyComparableValue(value) {
  if (value === null || value === undefined) {
    return true;
  }
  if (typeof value === "string") {
    return value.trim() === "";
  }
  if (Array.isArray(value)) {
    return value.length === 0 || value.every(emptyComparableValue);
  }
  if (typeof value === "object") {
    return Object.keys(value).length === 0;
  }
  return false;
}

function comparableValue(value) {
  if (typeof value === "string") {
    return value.trim();
  }
  return JSON.stringify(value);
}

function uniqueValues(values) {
  const seen = new Set();
  const result = [];
  for (const value of values) {
    const text = String(value ?? "").trim();
    if (!text || seen.has(text)) {
      continue;
    }
    seen.add(text);
    result.push(text);
  }
  return result;
}

function parseURL(value) {
  try {
    return new URL(value);
  } catch {
    return null;
  }
}
