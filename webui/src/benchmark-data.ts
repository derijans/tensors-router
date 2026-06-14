import type { BenchmarkSection, Model } from "./types";

export const benchmarkSections: BenchmarkSection[] = ["runtime", "llm", "embed", "image", "voice", "music"];

export function benchmarkCompactLabel(model: Model): string {
  const latest = model.benchmark?.latest;
  if (!latest) {
    return "none";
  }
  return `${latest.status} ${latest.duration_ms || 0}ms`;
}
