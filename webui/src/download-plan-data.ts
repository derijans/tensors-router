import type { DownloadPlan, PlannedFile } from "./types";

export function selectedDownloadFiles(plan: DownloadPlan, selectedPaths: string[]): PlannedFile[] {
  const selected = new Set(selectedPaths);
  return plan.files.filter(file => selected.has(file.path));
}

export function selectedDownloadBytes(plan: DownloadPlan, selectedPaths: string[]): number {
  return selectedDownloadFiles(plan, selectedPaths).reduce((total, file) => total + file.size, 0);
}

export function toggleDownloadPath(selectedPaths: string[], path: string): string[] {
  return selectedPaths.includes(path) ? selectedPaths.filter(value => value !== path) : [...selectedPaths, path];
}
