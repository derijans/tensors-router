import type { JsonObject, JsonValue } from "./types";

export function isJsonObject(value: unknown): value is JsonObject {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }
  return Object.values(value as Record<string, unknown>).every(isJsonValue);
}

export function isJsonValue(value: unknown): value is JsonValue {
  if (value === null) {
    return true;
  }
  switch (typeof value) {
    case "boolean":
    case "number":
    case "string":
      return true;
    case "object":
      return Array.isArray(value) ? value.every(isJsonValue) : isJsonObject(value);
    default:
      return false;
  }
}

export function jsonRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

export function cloneJsonObject<T extends JsonValue>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}
