import type { FieldGroup, Model, NodeInventory, OptionDefinition, Options } from "../../../types";
import { compareOptionKeys } from "../../../domain/option-keys";
import { optionSection } from "../../../domain/option-values";
import { parseURL } from "../../../lib/url";

export const sectionOrder = ["llm", "image", "embed", "voice", "music", "runtime", "other"];

export const sectionLabels: Record<string, string> = {
  llm: "LLM",
  image: "Image",
  embed: "Embed",
  voice: "Voice",
  music: "Music",
  runtime: "Runtime",
  other: "Other"
};

export const sectionModelKeys: Record<string, string[]> = {
  llm: ["model_param", "model"],
  image: ["sdmodel"],
  embed: ["embeddingsmodel", "mmproj"],
  voice: ["whispermodel", "ttsmodel", "ttswavtokenizer", "ttsdir"],
  music: ["musicllm", "musicembeddings", "musicdiffusion", "musicvae"]
};

export function groupedFieldKeys(fields: Options, definitionForKey: (key: string) => OptionDefinition | undefined): FieldGroup[] {
  const groups = new Map<string, string[]>(sectionOrder.map(section => [section, []]));
  for (const key of Object.keys(fields).sort(compareOptionKeys)) {
    const section = optionSection(definitionForKey(key));
    const keys = groups.get(section) ?? [];
    keys.push(key);
    groups.set(section, keys);
  }
  return sectionOrder
    .map(section => ({section, keys: groups.get(section) ?? []}))
    .filter(group => group.keys.length > 0);
}

export function defaultConfigForNode(node: NodeInventory | null): Options {
  const hardware = node?.hardware;
  const values: Options = {
    quiet: true,
    nomodel: false,
    contextsize: 4096,
    threads: hardware?.max_threads ? Math.max(1, Math.floor(hardware.max_threads / 2)) : -1,
    batchsize: 512,
    usemmap: true,
    usemlock: false,
    gpulayers: hardware?.gpu_backend && hardware.gpu_backend !== "cpu" && hardware.gpu_backend !== "unknown" ? -1 : 0
  };
  if (hardware?.gpu_backend === "cuda" || hardware?.gpu_backend === "rocm") {
    values.usecuda = true;
  }
  if (hardware?.gpu_backend === "vulkan") {
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

export function nodeLabel(node: NodeInventory): string {
  return `${node.node_id || "node"} / ${node.backend_mode || "backend"}`;
}

export function configLabel(model: Model): string {
  return `${model.local_id || model.public_id || "config"} / ${model.filename || ""}`;
}

export function suggestedConfigID(node: NodeInventory | null, fallback: string): string {
  const nodePrefix = (node?.node_id || "node").toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^-|-$/g, "");
  return `${nodePrefix || "node"}-${fallback}`;
}

export function safeID(value: string): string {
  return String(value).replace(/[^a-z0-9_-]/gi, "-");
}
