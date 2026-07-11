import { reviewConversions } from "./dialogs";
import { state } from "./state";
import type { ParseResult } from "./types";

export function recordConversion(scope: string, field: string, result: ParseResult): void {
  const key = `${scope}:${field}`;
  delete state.conversionWarnings[key];
  for (const warning of result.warnings) {
    state.conversionWarnings[key] = {...warning, field: conversionFieldLabel(scope, field)};
  }
  state.acceptedConversionSignature = "";
}

function conversionFieldLabel(scope: string, field: string): string {
  if (scope.startsWith("lane-")) {
    return `${scope.slice("lane-".length)}.${field}`;
  }
  return `${scope}.${field}`;
}

export function clearConversionWarnings(): void {
  state.conversionWarnings = {};
  state.acceptedConversionSignature = "";
}

export function discardConversion(scope: string, field: string): void {
  delete state.conversionWarnings[`${scope}:${field}`];
  state.acceptedConversionSignature = "";
}

export function clearConversionScope(scope: string): void {
  const prefix = `${scope}:`;
  for (const key of Object.keys(state.conversionWarnings)) {
    if (key.startsWith(prefix)) {
      delete state.conversionWarnings[key];
    }
  }
  state.acceptedConversionSignature = "";
}

export function clearConstructorConversions(): void {
  for (const scope of ["constructor", "lane-text", "lane-image", "lane-embeddings", "lane-voice", "lane-music"]) {
    clearConversionScope(scope);
  }
}

export function invalidateAcceptedConversions(): void {
  state.acceptedConversionSignature = "";
}

export async function confirmPendingConversions(scope: "quick" | "constructor"): Promise<boolean> {
  const warnings = Object.entries(state.conversionWarnings)
    .filter(([key]) => conversionBelongsToScope(key, scope))
    .map(([, warning]) => warning)
    .sort((left, right) => left.field.localeCompare(right.field));
  if (warnings.length === 0) {
    return true;
  }
  const signature = `${scope}:${JSON.stringify(warnings)}`;
  if (signature === state.acceptedConversionSignature) {
    return true;
  }
  if (!await reviewConversions(warnings)) {
    return false;
  }
  state.acceptedConversionSignature = signature;
  return true;
}

function conversionBelongsToScope(key: string, scope: "quick" | "constructor"): boolean {
  if (scope === "quick") {
    return key.startsWith("quick:");
  }
  return key.startsWith("constructor:") || key.startsWith("lane-");
}
