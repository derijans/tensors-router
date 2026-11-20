import { finiteOrZero, formatDecimal } from "./format-number";

export function formatDurationSeconds(value: number | undefined): string {
  const seconds = finiteOrZero(value);
  if (seconds < 60) {
    return `${formatDecimal(seconds, 1)}s`;
  }
  const minutes = seconds / 60;
  if (minutes < 60) {
    return `${formatDecimal(minutes, 1)}m`;
  }
  return `${formatDecimal(minutes / 60, 1)}h`;
}

export function formatClockDuration(totalSeconds: number): string {
  const seconds = Math.ceil(totalSeconds);
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = String(seconds % 60).padStart(2, "0");
  return hours > 0 ? `${hours}:${String(minutes).padStart(2, "0")}:${rest}` : `${minutes}:${rest}`;
}

export function formatElapsedSeconds(milliseconds: number): string {
  return `${(Math.max(0, milliseconds) / 1000).toFixed(1)}s`;
}

export function formatEpochTime(milliseconds: number): string {
  return milliseconds ? new Date(milliseconds).toLocaleString() : "never";
}

export function formatTimestamp(value: string): string {
  return validDate(value)?.toLocaleString() ?? value;
}

export function formatClockTime(value: string): string {
  return validDate(value)?.toLocaleTimeString() ?? value;
}

export function formatCalendarDate(value: string): string {
  return validDate(value)?.toISOString().slice(0, 10) ?? value;
}

export function timestampMillis(value: string): number | undefined {
  return value ? validDate(value)?.getTime() : undefined;
}

function validDate(value: string): Date | undefined {
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? undefined : parsed;
}
