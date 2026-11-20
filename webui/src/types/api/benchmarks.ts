import type { JsonValue } from "../json";

export type BenchmarkType = "general" | "section";

export type BenchmarkSection = "runtime" | "llm" | "embed" | "image" | "voice" | "music";

export interface BenchmarkMetric {
  name: string;
  status: string;
  duration_ms?: number;
  value?: number;
  unit?: string;
  error?: string;
}

export interface BenchmarkOptionChange {
  key: string;
  kind: string;
  previous?: JsonValue;
  current?: JsonValue;
}

export interface BenchmarkSummary {
  run_id: string;
  type: BenchmarkType;
  section: string;
  status: string;
  started_at: number;
  finished_at: number;
  duration_ms: number;
  metrics?: BenchmarkMetric[];
  error?: string;
  option_changes?: BenchmarkOptionChange[];
}

export interface ModelBenchmark {
  latest?: BenchmarkSummary;
  sections?: Record<string, BenchmarkSummary>;
}

export interface BenchmarkRecord extends ModelBenchmark {
  node_id: string;
  model_id: string;
  history?: BenchmarkSummary[];
}

export interface BenchmarkRunRequest {
  node_id?: string;
  model_id: string;
  type: BenchmarkType;
  sections?: string[];
  iterations?: number;
  timeout_seconds?: number;
}
