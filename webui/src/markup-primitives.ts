import { SafeHTML, displayText, emptyHTML, html } from "./safe-html";
import type { Accent, LaneAccent } from "./types";

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

export function accentClass(accent: Accent): string {
  return accent.startsWith("lane-") ? `lane-badge ${accent}` : `tone-${accent}`;
}

export function badge(label: unknown, accent: Accent): SafeHTML {
  const value = displayText(label).trim();
  if (!value) {
    return emptyHTML;
  }
  return html`<span class="badge ${accentClass(accent)}">${value}</span>`;
}

export function fact(label: string, value: unknown): SafeHTML {
  return html`<div class="fact"><dt>${label}</dt><dd>${displayText(value)}</dd></div>`;
}

export function optionElement(value: string, label: unknown, selected = false): SafeHTML {
  return html`<option value="${value}"${selected ? " selected" : ""}>${displayText(label)}</option>`;
}
