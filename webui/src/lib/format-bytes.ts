import { formatCount } from "./format-number";

const kibibyte = 1024;
const mebibyte = kibibyte * 1024;
const gibibyte = mebibyte * 1024;

export function formatBytes(value: number): string {
  if (value < kibibyte) return `${value} B`;
  if (value < mebibyte) return `${(value / kibibyte).toFixed(1)} KB`;
  if (value < gibibyte) return `${(value / mebibyte).toFixed(1)} MB`;
  return `${(value / gibibyte).toFixed(1)} GB`;
}

export function formatMegabytes(value: number | undefined): string {
  return `${formatCount(value)} MB`;
}

export function gigabyteFigure(megabytes: number): string {
  const gigabytes = Math.max(0, megabytes) / 1024;
  return gigabytes >= 100 ? gigabytes.toFixed(0) : gigabytes.toFixed(1);
}

export function formatGigabytes(megabytes: number): string {
  return `${gigabyteFigure(megabytes)} GB`;
}
