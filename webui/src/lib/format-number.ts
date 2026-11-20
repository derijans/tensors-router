export function finiteOrZero(value: number | undefined): number {
  return Number.isFinite(value ?? 0) ? value ?? 0 : 0;
}

export function formatCount(value: number | undefined): string {
  return Math.round(finiteOrZero(value)).toLocaleString("en-US");
}

export function formatDecimal(value: number | undefined, digits = 1): string {
  const safe = finiteOrZero(value);
  if (Number.isInteger(safe)) {
    return safe.toLocaleString("en-US");
  }
  return safe.toLocaleString("en-US", {
    maximumFractionDigits: digits,
    minimumFractionDigits: safe > 0 && safe < 10 ? digits : 0
  });
}

export function formatPercent(value: number | undefined): string {
  return `${formatDecimal(value, 1)}%`;
}

export function clampedPercent(part: number, total: number): number {
  return Math.min(100, Math.max(0, (part / total) * 100));
}

export function formatIntegerOrFixed(value: number, fractionDigits: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(fractionDigits);
}
