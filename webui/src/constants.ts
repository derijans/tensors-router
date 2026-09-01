import type { LaneKind } from "./types";

export interface LaneMetadata {
  label: string;
  shortLabel: string;
  section: string;
  accent: string;
  dropLabel: string;
}

export const laneKinds: LaneKind[] = ["text", "image", "embeddings", "voice", "music"];

export type BackendMode = "kobold" | "llama_sdcpp" | "vllm";
export type FamilyTrigger = "family:kobold" | "family:llama_sdcpp" | "family:vllm";
export type UnloadPolicy = "none" | LaneKind | "all" | FamilyTrigger;
export type UnloadTarget = Exclude<UnloadPolicy, "none" | FamilyTrigger>;

export const backendModeKey = "backend_mode";
export const unloadPolicyKey = "router_unload_policy";
export const jinjaKwargsKey = "jinja_kwargs";
export const jinjaKwargsPrecedenceKey = "router_jinja_kwargs_precedence";

export const backendModes: BackendMode[] = ["kobold", "llama_sdcpp", "vllm"];
export const familyTriggers: FamilyTrigger[] = ["family:kobold", "family:llama_sdcpp", "family:vllm"];
export const unloadPolicies: UnloadPolicy[] = ["none", ...laneKinds, "all", ...familyTriggers];
export const unloadTargets: UnloadTarget[] = [...laneKinds, "all"];

export const backendModeLabels: Record<BackendMode, string> = {
  kobold: "Kobold",
  llama_sdcpp: "llama/sd.cpp",
  vllm: "vLLM"
};

export const unloadPolicyLabels: Record<UnloadPolicy, string> = {
  none: "None",
  text: "Text",
  image: "Image",
  embeddings: "Embeddings",
  voice: "Voice",
  music: "Music",
  all: "All",
  "family:kobold": "Any Kobold config",
  "family:llama_sdcpp": "Any llama/sd.cpp config",
  "family:vllm": "Any vLLM config"
};

export const jinjaKwargsPrecedenceLabels: Record<"config" | "client", string> = {
  config: "Config wins",
  client: "Client wins"
};

export function compareOptionKeys(left: string, right: string): number {
  return optionSortKey(left).localeCompare(optionSortKey(right));
}

function optionSortKey(key: string): string {
  if (key === jinjaKwargsPrecedenceKey) {
    return `${jinjaKwargsKey}~`;
  }
  return key;
}

export const laneMetadata: Record<LaneKind, LaneMetadata> = {
  text: {
    label: "LLM",
    shortLabel: "Text",
    section: "llm",
    accent: "cyan",
    dropLabel: "Drop a text config or model file"
  },
  image: {
    label: "Image",
    shortLabel: "Image",
    section: "image",
    accent: "magenta",
    dropLabel: "Drop an image config or model file"
  },
  embeddings: {
    label: "Embed",
    shortLabel: "Embed",
    section: "embed",
    accent: "lime",
    dropLabel: "Drop an embedding config or model file"
  },
  voice: {
    label: "Voice",
    shortLabel: "Voice",
    section: "voice",
    accent: "amber",
    dropLabel: "Drop Whisper, TTS, tokenizer, or voice dir"
  },
  music: {
    label: "Music",
    shortLabel: "Music",
    section: "music",
    accent: "violet",
    dropLabel: "Drop Music LLM, embeddings, diffusion, or VAE"
  }
};

export function isLaneKind(value: string | undefined): value is LaneKind {
  return laneKinds.includes(value as LaneKind);
}
