import { normalizeModelHash, normalizeParameterRange } from "./download-finder-data";
import { downloadsElements } from "./downloads-elements";

export function normalizedExpectedHash(): string {
  return normalizeModelHash(downloadsElements.downloadExpectedHashInput.value.trim());
}

export function requestedFiles(): string[] {
  return downloadsElements.downloadFilesInput.value.split(/\r?\n/).map(value => value.trim()).filter(Boolean);
}

export function downloadToken(): string | undefined {
  const token = downloadsElements.downloadTokenInput.value.trim();
  return token || undefined;
}

export function parameterRange(): string | undefined {
  return normalizeParameterRange(downloadsElements.downloadParameterMin.value.trim(), downloadsElements.downloadParameterMax.value.trim());
}
