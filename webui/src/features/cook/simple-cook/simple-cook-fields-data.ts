import { backendModeKey } from "../../../domain/backend-mode";
import { jinjaKwargsKey, jinjaKwargsPrecedenceKey } from "../../../domain/option-keys";
import { optionSection, optionValueText } from "../../../domain/option-values";
import { unloadPolicyKey } from "../../../domain/unload-policy";
import { normalizedQuery } from "../../../lib/text";
import type { FieldGroup, JsonValue, OptionDefinition, Options } from "../../../types";
import { groupedFieldKeys, sectionLabels } from "./simple-cook-data";

export const primaryRuntimeKeys = [backendModeKey, unloadPolicyKey];

const virtualFieldDefaults: Record<string, JsonValue> = {
  [unloadPolicyKey]: "none",
  [jinjaKwargsPrecedenceKey]: "config"
};

export function simpleCookFieldGroups(fields: Options, definitionForKey: (key: string) => OptionDefinition | undefined): FieldGroup[] {
  const groups = groupedFieldKeys(fields, definitionForKey)
    .map(group => ({
      ...group,
      keys: withJinjaPrecedenceKey(group.keys.filter(key => !primaryRuntimeKeys.includes(key)))
    }))
    .filter(group => group.keys.length > 0);
  const runtimeGroup = groups.find(group => group.section === "runtime");
  if (runtimeGroup) {
    runtimeGroup.keys = [...primaryRuntimeKeys, ...runtimeGroup.keys];
    return groups;
  }
  return [...groups, {section: "runtime", keys: [...primaryRuntimeKeys]}];
}

export function isVirtualField(fields: Options, key: string): boolean {
  return (key === backendModeKey || key === jinjaKwargsPrecedenceKey) && !Object.hasOwn(fields, key);
}

export function effectiveFieldValue(fields: Options, key: string, backendMode: string): JsonValue | undefined {
  if (Object.hasOwn(fields, key)) {
    return fields[key];
  }
  if (key === backendModeKey) {
    return backendMode;
  }
  return virtualFieldDefaults[key];
}

export function fieldMatchesFilter(key: string, value: JsonValue | undefined, filter: string): boolean {
  const query = normalizedQuery(filter);
  return !query || `${key} ${optionValueText(value)}`.toLowerCase().includes(query);
}

export function addableFieldDefinitions(definitions: readonly OptionDefinition[], fields: Options): OptionDefinition[] {
  return definitions
    .filter(definition => !primaryRuntimeKeys.includes(definition.key) && !Object.hasOwn(fields, definition.key))
    .sort((left, right) => `${optionSection(left)}:${left.key}`.localeCompare(`${optionSection(right)}:${right.key}`));
}

export function addableFieldLabel(definition: OptionDefinition): string {
  return `${sectionLabels[optionSection(definition)] || "Other"} / ${definition.key}`;
}

export function fieldCountLabel(count: number): string {
  return count === 1 ? "1 field" : `${count} fields`;
}

function withJinjaPrecedenceKey(keys: string[]): string[] {
  const jinjaIndex = keys.indexOf(jinjaKwargsKey);
  if (jinjaIndex < 0) {
    return keys;
  }
  const values = keys.filter(key => key !== jinjaKwargsPrecedenceKey);
  values.splice(jinjaIndex + 1, 0, jinjaKwargsPrecedenceKey);
  return values;
}
