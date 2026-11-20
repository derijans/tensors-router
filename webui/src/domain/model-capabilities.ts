import type { Model } from "../types";

export const modelCapabilities = ["llm", "image", "embeddings", "multimodal", "voice", "music"] as const;

export type ModelCapability = typeof modelCapabilities[number];

const capabilityFlags: Record<ModelCapability, (model: Model) => boolean> = {
  llm: model => model.has_llm,
  image: model => model.has_image,
  embeddings: model => model.has_embeddings,
  multimodal: model => model.has_multimodal,
  voice: model => model.has_voice,
  music: model => model.has_music
};

export function modelHasCapability(model: Model, capability: string): boolean {
  return Object.hasOwn(capabilityFlags, capability) && capabilityFlags[capability as ModelCapability](model);
}

export function modelCapabilityNames(model: Model): ModelCapability[] {
  return modelCapabilities.filter(capability => capabilityFlags[capability](model));
}
