import type { BenchmarkMetric, BenchmarkSection, BenchmarkSummary, Model } from "./types";

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
