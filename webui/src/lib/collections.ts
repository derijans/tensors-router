export function uniqueSorted(values: readonly string[]): string[] {
  return [...new Set(values)].sort((left, right) => left.localeCompare(right));
}

export function groupBy<T>(items: readonly T[], key: (item: T) => string): Map<string, T[]> {
  const groups = new Map<string, T[]>();
  for (const item of items) {
    const groupKey = key(item);
    const group = groups.get(groupKey);
    if (group) group.push(item);
    else groups.set(groupKey, [item]);
  }
  return groups;
}

export function lookupOr<Value>(table: Readonly<Record<string, Value>>, key: string, fallback: NoInfer<Value>): Value {
  return (Object.hasOwn(table, key) ? table[key] : undefined) ?? fallback;
}

export function uniqueSortedNonBlank(values: readonly string[]): string[] {
  return uniqueSorted(values.map(value => value.trim()).filter(Boolean));
}

export function toggledMember<T>(items: readonly T[], item: T): T[] {
  return items.includes(item) ? items.filter(existing => existing !== item) : [...items, item];
}
