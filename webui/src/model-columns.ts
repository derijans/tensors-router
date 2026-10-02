export const modelColumns = [
  {key: "id", label: "ID", shownByDefault: true},
  {key: "node", label: "Node", shownByDefault: true},
  {key: "enabled", label: "Enabled", shownByDefault: false},
  {key: "backend", label: "Backend", shownByDefault: true},
  {key: "capabilities", label: "Capabilities", shownByDefault: true},
  {key: "options", label: "Options", shownByDefault: false},
  {key: "benchmark", label: "Benchmark", shownByDefault: false},
  {key: "available", label: "Available", shownByDefault: true},
  {key: "routing", label: "Routing", shownByDefault: true},
  {key: "separate", label: "Separate", shownByDefault: false},
  {key: "actions", label: "Actions", shownByDefault: true}
] as const;

export type ModelColumnKey = typeof modelColumns[number]["key"];

export type ModelColumnChoices = Partial<Record<ModelColumnKey, boolean>>;

const widthForEveryColumnPixels = 1500;

export function hasRoomForEveryColumn(tableWidthPixels: number): boolean {
  return tableWidthPixels >= widthForEveryColumnPixels;
}

export function visibleModelColumns(choices: ModelColumnChoices, roomForEveryColumn: boolean): ModelColumnKey[] {
  return modelColumns
    .filter(column => choices[column.key] ?? (roomForEveryColumn || column.shownByDefault))
    .map(column => column.key);
}

export function isModelColumnKey(value: string | undefined): value is ModelColumnKey {
  return modelColumns.some(column => column.key === value);
}

export function parsedModelColumnChoices(value: unknown): ModelColumnChoices {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return {};
  }
  const choices: ModelColumnChoices = {};
  for (const [key, shown] of Object.entries(value)) {
    if (isModelColumnKey(key) && typeof shown === "boolean") {
      choices[key] = shown;
    }
  }
  return choices;
}
