import type { LaneAccent, LaneKind } from "../types";

export interface LaneMetadata {
  label: string;
  shortLabel: string;
  section: string;
  dropLabel: string;
}

export const laneKinds: LaneKind[] = ["text", "image", "embeddings", "voice", "music"];

export const laneMetadata: Record<LaneKind, LaneMetadata> = {
  text: {
    label: "LLM",
    shortLabel: "Text",
    section: "llm",
    dropLabel: "Drop a text config or model file"
  },
  image: {
    label: "Image",
    shortLabel: "Image",
    section: "image",
    dropLabel: "Drop an image config or model file"
  },
  embeddings: {
    label: "Embed",
    shortLabel: "Embed",
    section: "embed",
    dropLabel: "Drop an embedding config or model file"
  },
  voice: {
    label: "Voice",
    shortLabel: "Voice",
    section: "voice",
    dropLabel: "Drop Whisper, TTS, tokenizer, or voice dir"
  },
  music: {
    label: "Music",
    shortLabel: "Music",
    section: "music",
    dropLabel: "Drop Music LLM, embeddings, diffusion, or VAE"
  }
};

export function isLaneKind(value: string | undefined): value is LaneKind {
  return laneKinds.includes(value as LaneKind);
}

const laneAccentByName: Record<string, LaneAccent> = {
  text: "lane-text",
  llm: "lane-text",
  image: "lane-image",
  voice: "lane-voice",
  speech: "lane-voice",
  transcription: "lane-voice",
  embed: "lane-embed",
  embeddings: "lane-embed",
  music: "lane-music"
};

export function laneAccent(lane: string): LaneAccent {
  return laneAccentByName[lane.trim().toLowerCase()] ?? "lane-other";
}
