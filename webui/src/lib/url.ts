export function parseURL(value: string): URL | null {
  try {
    return new URL(value);
  } catch {
    return null;
  }
}
