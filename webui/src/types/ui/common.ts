import type { JsonValue } from "../json";

export type OverviewPeriod = "24h" | "7d" | "30d";

export type Tone = "accent" | "info" | "success" | "warning" | "danger" | "neutral";

export type LaneAccent = "lane-text" | "lane-image" | "lane-voice" | "lane-embed" | "lane-music" | "lane-other";

export type Accent = Tone | LaneAccent;

export interface ChartLayer {
  key: string;
  label: string;
  accent: Accent;
  values: number[];
}

export interface StackedSeries {
  layers: ChartLayer[];
  bucketStarts: number[];
}

export interface SelectChoice {
  value: string;
  label: string;
}

export interface ConversionWarning {
  field: string;
  original: string;
  proposed: JsonValue;
  reason: string;
}

export interface ParseResult {
  value: JsonValue;
  warnings: ConversionWarning[];
}

export interface OperationStatus {
  key: string;
  group: string;
  label: string;
  pending: boolean;
  error: string;
  retry?: () => Promise<unknown>;
}

export type RefreshInventory = () => Promise<void>;

export type MemorySegmentAccent = LaneAccent | "lane-lent";

export interface MemorySegment {
  accent: MemorySegmentAccent;
  percent: number;
  label: string;
}
