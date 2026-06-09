import type { LaneKind } from "./types";

export interface LaneMetadata {
  label: string;
  shortLabel: string;
  section: string;
  accent: string;
  dropLabel: string;
}

export const laneKinds: LaneKind[] = ["text", "image", "embeddings", "voice", "music"];

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
