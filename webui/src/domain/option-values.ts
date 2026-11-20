import { isJsonValue } from "../lib/json";
import type { JsonValue, OptionDefinition, Options, ParseResult } from "../types";

export function optionSummary(options: Record<string, JsonValue> | undefined): string {
  const count = Object.keys(options ?? {}).length;
  return count ? `${count} filled` : "none";
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

export function threadCountOption(key: string, definition: OptionDefinition | undefined): boolean {
  if (definition) {
    return definition.value_type === "number" && definition.key.endsWith("threads");
  }
  return String(key || "").trim().toLowerCase().endsWith("threads");
}

export function optionValueText(value: JsonValue | undefined): string {
  if (typeof value === "string") {
    return value;
  }
  if (value === undefined) {
    return "";
  }
  return JSON.stringify(value) ?? "";
}

export function optionInputList(value: JsonValue | undefined): string[] {
  if (Array.isArray(value)) {
    return value.filter((item): item is string => typeof item === "string" && item.trim() !== "");
  }
  if (typeof value === "string" && value.trim() !== "") {
    return value.split(/[\s,]+/).filter(Boolean);
  }
  return [];
}

export function parseOptionInput(definition: OptionDefinition | undefined, value: string): ParseResult {
  const trimmed = value.trim();
  switch (definition?.value_type) {
    case "bool": {
      const normalized = trimmed.toLowerCase();
      const proposed = normalized === "true" || normalized === "1" || normalized === "yes";
      if (normalized === "true" || normalized === "false") {
        return {value: proposed, warnings: []};
      }
      return conversionResult(definition.key, value, proposed, "The value is converted to a boolean.");
    }
    case "number": {
      const number = Number(trimmed);
      if (trimmed && Number.isFinite(number)) {
        return {value: number, warnings: []};
      }
      return conversionResult(definition.key, value, 0, "The value is not a finite number and is converted to 0.");
    }
    case "json":
      if (!trimmed) {
        return conversionResult(definition.key, value, {}, "Blank JSON is converted to an empty object.");
      }
      try {
        const parsed = JSON.parse(trimmed) as unknown;
        if (isJsonValue(parsed)) {
          return {value: parsed, warnings: []};
        }
        return conversionResult(definition.key, value, value, "The parsed value is not supported JSON and is kept as a string.");
      } catch {
        return conversionResult(definition.key, value, value, "Invalid JSON is kept as a string.");
      }
    case "string_list": {
      if (!trimmed) {
        return {value: [], warnings: []};
      }
      return {value: trimmed.split(/[\s,]+/).filter(Boolean), warnings: []};
    }
    case "string":
      return {value, warnings: []};
    default:
      return {value, warnings: []};
  }
}

function conversionResult(field: string, original: string, proposed: JsonValue, reason: string): ParseResult {
  return {
    value: proposed,
    warnings: [{field, original, proposed, reason}]
  };
}

export function emptyComparableValue(value: JsonValue | undefined): boolean {
  if (value === null || value === undefined) {
    return true;
  }
  if (typeof value === "string") {
    return value.trim() === "";
  }
  if (Array.isArray(value)) {
    return value.every(emptyComparableValue);
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

export function cloneOptions(options: Options | undefined): Options {
  return structuredClone(options ?? {});
}

export function optionSection(definition: OptionDefinition | undefined): string {
  return definition?.section || "other";
}

export function defaultFieldValue(definition: OptionDefinition | undefined): JsonValue {
  if (definition?.default !== undefined && definition.default !== "") {
    return parseOptionInput(definition, definition.default).value;
  }
  return emptyValueForType(definition);
}

export function emptyValueForType(definition: OptionDefinition | undefined): JsonValue {
  switch (definition?.value_type) {
    case "bool":
      return false;
    case "number":
      return 0;
    case "json":
      return {};
    default:
      return "";
  }
}
