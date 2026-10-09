import { elements } from "./elements";
import { normalizeModelHash, normalizeParameterRange } from "./download-finder-data";

export function normalizedExpectedHash(): string {
  return normalizeModelHash(elements.downloadExpectedHashInput.value.trim());
}

export function requestedFiles(): string[] {
  return elements.downloadFilesInput.value.split(/\r?\n/).map(value => value.trim()).filter(Boolean);
}

export function downloadToken(): string | undefined {
  const token = elements.downloadTokenInput.value.trim();
  return token || undefined;
}

export function parameterRange(): string | undefined {
  return normalizeParameterRange(elements.downloadParameterMin.value.trim(), elements.downloadParameterMax.value.trim());
}
