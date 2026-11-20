import { formatMegabytes } from "../../../lib/format-bytes";
import { formatCount, formatDecimal, formatPercent } from "../../../lib/format-number";
import { formatDurationSeconds } from "../../../lib/format-time";
import type { AnalyticsRecentEvent } from "../../../types";
import { failureDetail, queueWaitDetail } from "./analytics-failure-detail-data";

type EventDetail = (event: AnalyticsRecentEvent) => string;

const sectionDetails = new Map<string, EventDetail>([
  ["image", imageDetail],
  ["embed", embeddingDetail],
  ["voice", audioDetail],
  ["music", audioDetail]
]);

export function recentEventDetail(event: AnalyticsRecentEvent, showStreamMetrics: boolean): string {
  const failure = failureDetail(event);
  if (failure) {
    return failure;
  }
  if (event.event_type === "model_load") {
    return loadDetail(event);
  }
  if (showStreamMetrics) {
    return streamDetail(event);
  }
  return (sectionDetails.get(event.section) ?? tokenDetail)(event);
}

function streamDetail(event: AnalyticsRecentEvent): string {
  const parts = [
    event.section === "llm" ? tokenCounts(event) : "",
    queueWaitDetail(event),
    event.ttft_ms ? `TTFT ${formatDecimal(event.ttft_ms, 0)}ms` : "",
    ...decodeDetails(event),
    event.max_gap_ms ? `max gap ${formatDecimal(event.max_gap_ms, 0)}ms` : "",
    event.finish_reason ?? "",
    event.aborted ? "aborted" : ""
  ].filter(Boolean);
  return parts.length === 0 ? "no stream metrics" : parts.join(" / ");
}

function decodeDetails(event: AnalyticsRecentEvent): string[] {
  if (!event.decode_ms) {
    return [];
  }
  const intervals = (event.output_tokens ?? 0) - 1;
  const decode = `decode ${formatDecimal(event.decode_ms, 0)}ms`;
  return intervals > 0 ? [decode, `${formatDecimal((intervals * 1000) / event.decode_ms, 1)} tok/s decode`] : [decode];
}

function tokenDetail(event: AnalyticsRecentEvent): string {
  const speed = event.tokens_per_second ? ` / ${formatDecimal(event.tokens_per_second, 1)} tok/s` : "";
  const promptSpeed = event.prompt_tokens_per_second ? ` / ${formatDecimal(event.prompt_tokens_per_second, 0)} tok/s prompt` : "";
  return `${tokenCounts(event)}${speed}${promptSpeed}${workVRAMDetail(event)}`;
}

function tokenCounts(event: AnalyticsRecentEvent): string {
  return `${formatCount(event.input_tokens)} in / ${formatCount(event.output_tokens)} out`;
}

function embeddingDetail(event: AnalyticsRecentEvent): string {
  return `${formatCount(event.input_tokens)} in / ${formatCount(event.embedding_count)} vectors${workVRAMDetail(event)}`;
}

function imageDetail(event: AnalyticsRecentEvent): string {
  const resolution = event.image_width && event.image_height ? ` / ${event.image_width}x${event.image_height}` : "";
  const type = event.image_type ? `${event.image_type} / ` : "";
  return `${type}${formatCount(event.image_count)} images${resolution}${workVRAMDetail(event)}`;
}

function audioDetail(event: AnalyticsRecentEvent): string {
  const metadata = [event.audio_language, event.audio_task].filter(Boolean).join(" / ");
  const prefix = metadata ? `${metadata} / ` : "";
  return `${prefix}${formatDurationSeconds(event.audio_seconds)} / ${formatCount(event.audio_tokens)} tokens${workVRAMDetail(event)}`;
}

function loadDetail(event: AnalyticsRecentEvent): string {
  const config = event.config_filename ? `${event.config_filename} / ` : "";
  return `${config}${formatDecimal(event.duration_ms, 0)}ms / ${formatMegabytes(event.load_vram_before_mb)} -> ${formatMegabytes(event.load_vram_after_mb)} / +${formatMegabytes(event.load_vram_delta_mb)}`;
}

function workVRAMDetail(event: AnalyticsRecentEvent): string {
  if (!event.work_vram_max_mb && !event.model_vram_estimate_mb) {
    return "";
  }
  return ` / VRAM ${formatMegabytes(event.work_vram_max_mb)} (${formatPercent(event.vram_peak_percent)}) / model ${formatMegabytes(event.model_vram_estimate_mb)}`;
}
