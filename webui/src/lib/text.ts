export function pluralSuffix(count: number): string {
  return count === 1 ? "" : "s";
}

export function truncate(value: string, limit: number): string {
  return value.length > limit ? `${value.slice(0, limit - 1)}…` : value;
}

export function normalizedQuery(query: string): string {
  return query.trim().toLowerCase();
}

export function isNonBlank(value: string): boolean {
  return value.trim() !== "";
}

export function matchesQuery(text: string, query: string): boolean {
  const normalized = normalizedQuery(query);
  return !normalized || text.toLowerCase().includes(normalized);
}
