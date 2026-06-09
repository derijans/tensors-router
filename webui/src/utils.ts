import { isJsonValue } from "./json";
import type { CookComponent, FileRecord, JsonValue, Model, OptionDefinition, ValidationIssue } from "./types";

export function escapeHTML(value: unknown): string {
  const entities: Record<string, string> = {
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    "\"": "&quot;",
    "'": "&#39;"
  };
  return displayText(value).replace(/[&<>"']/g, character => entities[character] ?? character);
}

export function escapeAttribute(value: unknown): string {
  return escapeHTML(value).replace(/`/g, "&#96;");
}

export function statusItem(label: string, value: string): string {
  return `
    <div class="status-item">
      <div class="status-label">${escapeHTML(label)}</div>
      <div class="status-value">${escapeHTML(value)}</div>
    </div>
  `;
}

export function chip(label: unknown, color: string): string {
  const value = displayText(label).trim();
  if (!value) {
    return "";
  }
  return `<span class="chip ${escapeAttribute(color)}">${escapeHTML(value)}</span>`;
}

export function renderIssue(item: ValidationIssue): string {
  return `
    <div class="issue ${item.severity === "error" ? "error" : ""}">
      <strong>${escapeHTML(item.severity)} / ${escapeHTML(item.code)}</strong>
      <span>${escapeHTML(item.message)}</span>
    </div>
  `;
}

export function issue(severity: "warning" | "error", code: string, message: string, field: string): ValidationIssue {
  return {severity, code, message, field};
}

export function kindColor(kind: string): string {
  switch (kind) {
    case "image":
      return "magenta";
    case "embeddings":
      return "lime";
    case "voice":
      return "amber";
    case "music":
      return "violet";
    default:
      return "cyan";
  }
}

export function capabilities(model: Model): string {
  const values: string[] = [];
  if (model.has_llm) values.push("llm");
  if (model.has_image) values.push("image");
  if (model.has_embeddings) values.push("embeddings");
  if (model.has_multimodal) values.push("multimodal");
  if (model.has_voice) values.push("voice");
  if (model.has_music) values.push("music");
  return values.join(", ") || "none";
}

export function optionSummary(options: Record<string, JsonValue> | undefined): string {
  const count = Object.keys(options ?? {}).length;
  return count ? `${count} filled` : "none";
}

export function fileRoles(file: FileRecord): string[] {
  return file.roles ?? [file.role || "unknown"];
}

export function hasKind(components: CookComponent[], kind: string): boolean {
  return components.some(component => component.kind === kind);
}

export function gpuOptionKey(key: string): boolean {
  const lowered = String(key).toLowerCase();
  return ["gpulayers", "tensor_split", "maingpu", "usecuda", "usecublas", "embeddingsgpu", "sdclipgpu", "sdflashattention"].includes(key) ||
    lowered.includes("gpu") ||
    lowered.includes("cuda");
}

export function highGPUOption(key: string, value: JsonValue | undefined): boolean {
  if (["gpulayers", "maingpu"].includes(key)) {
    return numberOption(value) > 0;
  }
  if (["tensor_split", "usecuda", "usecublas", "embeddingsgpu", "sdclipgpu", "sdflashattention"].includes(key)) {
    return truthy(value);
  }
  return gpuOptionKey(key) && truthy(value);
}

export function truthy(value: JsonValue | undefined): boolean {
  if (typeof value === "boolean") return value;
  if (typeof value === "number") return value !== 0;
  if (typeof value === "string") return value.trim() !== "";
  return value !== null && value !== undefined;
}

export function numberOption(value: JsonValue | undefined): number {
  if (typeof value === "number") return value;
  if (typeof value === "string") {
    const parsed = Number.parseInt(value, 10);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

export function optionInputValue(value: JsonValue | undefined): string {
  if (typeof value === "string") {
    return value;
  }
  if (value === undefined) {
    return "";
  }
  return JSON.stringify(value) ?? "";
}

export function optionValueLabel(value: JsonValue | undefined): string {
  if (typeof value === "string") {
    return value;
  }
  if (value === undefined) {
    return "";
  }
  return JSON.stringify(value) ?? "";
}

export function parseOptionInput(definition: OptionDefinition | undefined, value: string): JsonValue {
  const trimmed = value.trim();
  switch (definition?.value_type) {
    case "bool":
      return trimmed === "true" || trimmed === "1" || trimmed === "yes";
    case "number": {
      const number = Number(trimmed);
      return Number.isFinite(number) ? number : 0;
    }
    case "json":
      if (!trimmed) {
        return {};
      }
      try {
        const parsed = JSON.parse(trimmed) as unknown;
        return isJsonValue(parsed) ? parsed : value;
      } catch {
        return value;
      }
    default:
      return value;
  }
}

export function emptyComparableValue(value: JsonValue | undefined): boolean {
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

export function comparableValue(value: JsonValue | undefined): string {
  if (typeof value === "string") {
    return value.trim();
  }
  return JSON.stringify(value) ?? "";
}

export function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`;
  return `${(value / 1024 / 1024 / 1024).toFixed(1)} GB`;
}

function displayText(value: unknown): string {
  if (value === null || value === undefined) {
    return "";
  }
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") {
    return value.toString();
  }
  const json = JSON.stringify(value);
  return json ?? "";
}
