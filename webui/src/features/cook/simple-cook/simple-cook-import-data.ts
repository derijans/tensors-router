import type { Options } from "../../../types";
import { safeID } from "./simple-cook-data";

export function importedConfigNameStem(filename: string): string {
  const base = filename.replace(/\.(kcpps|json)$/i, "");
  return safeID(base) || "imported-config";
}

export function parseImportedOptions(content: string): Options {
  let parsed: unknown;
  try {
    parsed = JSON.parse(content);
  } catch {
    throw new Error("Selected file is not valid JSON");
  }
  if (!isOptionsObject(parsed)) {
    throw new Error("Selected file is not a KCPPS config (expected a JSON object)");
  }
  return parsed;
}

function isOptionsObject(value: unknown): value is Options {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
