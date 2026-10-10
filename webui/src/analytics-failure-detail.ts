import type { AnalyticsRecentEvent } from "./types";

const failureDetailLimit = 160;

export function failureDetail(event: AnalyticsRecentEvent): string {
  if (event.success || !event.error_message) {
    return "";
  }
  const upstream = event.upstream_status ? `backend ${event.upstream_status}: ` : "";
  const text = `${upstream}${event.error_message}`;
  return text.length > failureDetailLimit ? `${text.slice(0, failureDetailLimit - 1)}…` : text;
}

export function queueWaitDetail(event: AnalyticsRecentEvent): string {
  return event.queue_wait_ms ? `queued ${event.queue_wait_ms}ms` : "";
}
