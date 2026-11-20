import type { LaneKind } from "../types";
import { laneKinds } from "./lanes";

export type FamilyTrigger = "family:kobold" | "family:llama_sdcpp" | "family:vllm";

export type UnloadPolicy = "none" | LaneKind | "all" | FamilyTrigger;

export type UnloadTarget = Exclude<UnloadPolicy, "none" | FamilyTrigger>;

export const unloadPolicyKey = "router_unload_policy";

export const familyTriggers: FamilyTrigger[] = ["family:kobold", "family:llama_sdcpp", "family:vllm"];

export const unloadPolicies: UnloadPolicy[] = ["none", ...laneKinds, "all", ...familyTriggers];

export const unloadTargets: UnloadTarget[] = [...laneKinds, "all"];

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
