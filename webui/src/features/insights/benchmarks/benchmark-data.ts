import type { BenchmarkMetric, BenchmarkSection, BenchmarkSummary, Model, Tone } from "../../../types";
import { formatIntegerOrFixed } from "../../../lib/format-number";
import { lookupOr } from "../../../lib/collections";

export const benchmarkSections: BenchmarkSection[] = ["runtime", "llm", "embed", "image", "voice", "music"];

export function benchmarkCompactLabel(model: Model): string {
  const latest = model.benchmark?.latest;
  if (!latest) {
    return "none";
  }
  const tokensPerSecond = benchmarkMetric(latest, "tokens_per_second");
  if (tokensPerSecond?.value) {
    return `${latest.status} ${tokensPerSecond.value.toFixed(1)} tok/s`;
  }
  return `${latest.status} ${latest.duration_ms || 0}ms`;
}

export function benchmarkMetric(summary: BenchmarkSummary, name: string): BenchmarkMetric | null {
  return summary.metrics?.find(metric => metric.name === name) ?? null;
}

const benchmarkStatusTones: Record<string, Tone> = {
  success: "success",
  failed: "danger",
  partial: "warning"
};

export function benchmarkStatusTone(status: string): Tone {
  return lookupOr(benchmarkStatusTones, status, "neutral");
}

export function formatMetricValue(metric: NonNullable<BenchmarkSummary["metrics"]>[number]): string {
  if (metric.duration_ms) {
    return `${metric.duration_ms}ms`;
  }
  if (metric.value !== undefined && metric.unit) {
    return `${formatIntegerOrFixed(metric.value, 2)} ${metric.unit}`;
  }
  if (metric.value !== undefined) {
    return formatIntegerOrFixed(metric.value, 2);
  }
  return metric.status;
}

export function modelKey(model: Model | undefined): string {
  if (!model) {
    return "";
  }
  return `${model.node_id}\n${model.local_id}`;
}

export function modelLabel(model: Model): string {
  return `${model.node_id || "node"} / ${model.local_id || model.public_id}`;
}

export function isBenchmarkSection(value: string): value is BenchmarkSection {
  return benchmarkSections.includes(value as BenchmarkSection);
}
