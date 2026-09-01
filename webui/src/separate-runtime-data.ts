import type { SeparateRuntimeCandidates, SeparateRuntimeSettings } from "./types";

export type TriggerKind = "lane" | "family" | "config" | "special";

export function triggerKind(trigger: string): TriggerKind {
  if (trigger === "none" || trigger === "all") {
    return "special";
  }
  if (trigger.startsWith("family:")) {
    return "family";
  }
  if (trigger.startsWith("config:")) {
    return "config";
  }
  return "lane";
}

export interface TriggerGroups {
  lanes: string[];
  families: string[];
  configs: string[];
}

export function groupTriggersByKind(candidates: SeparateRuntimeCandidates): TriggerGroups {
  return {
    lanes: [...candidates.lanes],
    families: [...candidates.families],
    configs: [...candidates.configs]
  };
}

export function selectedTriggerKeys(settings: SeparateRuntimeSettings): Set<string> {
  return new Set(settings.triggers.filter(trigger => trigger !== "none" && trigger !== "all"));
}

export function doNotUnloadSelected(settings: SeparateRuntimeSettings): boolean {
  return settings.triggers.length === 0 || settings.triggers.includes("none");
}

export function separateButtonLabel(hasOverride: boolean, settings: SeparateRuntimeSettings): string {
  if (!hasOverride || !settings.run_separate) {
    return "Separate";
  }
  const triggers = settings.triggers.filter(trigger => trigger !== "none");
  if (triggers.length === 0) {
    return "Separate · on";
  }
  if (triggers.length === 1) {
    return "Separate · on · 1 trigger";
  }
  return `Separate · on · ${triggers.length} triggers`;
}

/**
 * "none" and "all" are absorbing: ticking either clears every other trigger, and
 * ticking any specific trigger clears them. Mirrors changedNodeSelection.
 */
export function changedTriggerSelection(selected: Set<string>, trigger: string, checked: boolean): Set<string> {
  const next = new Set(selected);
  if (!checked) {
    next.delete(trigger);
    return next;
  }
  if (trigger === "none" || trigger === "all") {
    return new Set([trigger]);
  }
  next.delete("none");
  next.delete("all");
  next.add(trigger);
  return next;
}

export function triggersFromSelection(selected: Set<string>): string[] {
  const triggers = [...selected];
  if (triggers.length === 0) {
    return ["none"];
  }
  if (triggers.includes("all")) {
    return ["all"];
  }
  if (triggers.includes("none")) {
    return ["none"];
  }
  return triggers.sort();
}
