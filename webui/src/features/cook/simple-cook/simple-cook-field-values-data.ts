import type { FieldRenderContext, Model, OptionDefinition, Options, SidebarValueRow } from "../../../types";
import { fileRoles } from "../../../domain/model-files";
import { comparableValue, emptyComparableValue, optionSection, optionValueText } from "../../../domain/option-values";
import { configLabel, sectionModelKeys } from "./simple-cook-data";

export function fieldChoices(key: string, definition: OptionDefinition | undefined, context: FieldRenderContext): string[] {
  const choices = [
    ...(definition?.choices ?? []),
    ...modelChoicesForDefinition(definition, context),
    ...observedValuesForField(key, context)
  ];
  return uniqueValues(choices.map(choice => inputChoiceValue(choice, definition)));
}

export function comparisonClass(key: string, section: string, context: FieldRenderContext): string {
  const currentValue = context.currentFields[key];
  const currentModelValue = modelCohortValue(context.currentFields, section);
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

export function sidebarValueRows(key: string, type: string, definitionForKey: (key: string) => OptionDefinition | undefined, context: FieldRenderContext): SidebarValueRow[] {
  const section = optionSection(definitionForKey(key));
  const models = type === "model" ? comparableModels(section, context) : context.otherNodeModels;
  const rows: SidebarValueRow[] = [];
  const seen = new Set<string>();
  for (const model of models) {
    const value = model.options?.[key];
    if (emptyComparableValue(value)) {
      continue;
    }
    const label = optionValueText(value);
    const seenKey = `${label}\n${model.local_id}`;
    if (seen.has(seenKey)) {
      continue;
    }
    seen.add(seenKey);
    rows.push({value: label, config: configLabel(model)});
  }
  return rows;
}

function modelChoicesForDefinition(definition: OptionDefinition | undefined, context: FieldRenderContext): string[] {
  if (!definition?.model_role) {
    return [];
  }
  const files = context.nodeFiles
    .filter(file => roleMatchesDefinition(fileRoles(file), definition.model_role ?? ""))
    .map(file => file.path);
  const models = context.nodeModels.flatMap(model => modelPathsForRole(model, definition.model_role ?? ""));
  return [...files, ...models];
}

function observedValuesForField(key: string, context: FieldRenderContext): string[] {
  return context.nodeModels
    .map(model => model.options?.[key])
    .filter(value => !emptyComparableValue(value))
    .map(optionValueText);
}

function comparableModels(section: string, context: FieldRenderContext): Model[] {
  const existing = context.comparableBySection.get(section);
  if (existing) {
    return existing;
  }
  const currentModelValue = modelCohortValue(context.currentFields, section);
  let models = context.otherNodeModels;
  if (!currentModelValue) {
    context.comparableBySection.set(section, models);
    return models;
  }
  models = models.filter(model => modelCohortValue(model.options ?? {}, section) === currentModelValue);
  context.comparableBySection.set(section, models);
  return models;
}

function modelCohortValue(options: Options | undefined, section: string): string {
  for (const key of sectionModelKeys[section] ?? []) {
    const value = options?.[key];
    if (!emptyComparableValue(value)) {
      return comparableValue(value);
    }
  }
  return "";
}

function roleMatchesDefinition(roles: string[], role: string): boolean {
  if (role === "llm") return roles.includes("llm");
  if (role === "image") return roles.includes("image");
  if (role === "embeddings") return roles.includes("embeddings") || roles.includes("llm");
  if (role === "multimodal") return roles.includes("multimodal");
  if (role === "vae") return roles.includes("vae");
  if (role === "clip") return roles.includes("clip");
  if (role === "t5") return roles.includes("t5");
  if (role === "upscaler") return roles.includes("upscaler");
  if (role === "lora") return roles.includes("lora");
  if (role === "voice") return roles.includes("voice");
  if (role === "music") return roles.includes("music");
  return true;
}

const modelPathReaders = new Map<string, (model: Model) => Array<string | undefined>>(Object.entries({
  llm: model => [typeof model.filename === "string" ? model.filename : undefined],
  image: model => [model.capabilities?.image?.model],
  embeddings: model => [model.capabilities?.embeddings?.model],
  multimodal: model => [model.capabilities?.multimodal?.projector],
  vae: model => [model.capabilities?.image?.vae],
  clip: model => {
    const image = model.capabilities?.image;
    return [image?.clip1, image?.clip2, image?.clip_l, image?.clip_g];
  },
  t5: model => [model.capabilities?.image?.t5xxl],
  upscaler: model => [model.capabilities?.image?.upscaler],
  lora: model => model.capabilities?.image?.lora ?? [],
  voice: model => {
    const voice = model.capabilities?.voice;
    return [voice?.whisper_model, voice?.tts_model, voice?.wav_tokenizer, voice?.directory];
  },
  music: model => {
    const music = model.capabilities?.music;
    return [music?.llm, music?.embeddings, music?.diffusion, music?.vae];
  }
}));

function modelPathsForRole(model: Model, role: string): string[] {
  const readPaths = modelPathReaders.get(role);
  return (readPaths ? readPaths(model) : []).filter((value): value is string => Boolean(value));
}

function inputChoiceValue(value: string, definition: OptionDefinition | undefined): string {
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

function uniqueValues(values: string[]): string[] {
  const seen = new Set<string>();
  const result: string[] = [];
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
