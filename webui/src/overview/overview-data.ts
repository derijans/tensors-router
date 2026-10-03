import { formatCount, formatDecimal, promptSpeedPrefix } from "../analytics-data";
import { clusterBuildVersion } from "../node-card-view";
import { formatGigabytes, gigabyteFigure } from "../node-memory-data";
import { filledBucketStarts } from "../lane-series";
import type {
  AnalyticsRecentEvent,
  AnalyticsResponse,
  AnalyticsTimeline,
  LoadCaptureAttempt,
  LoadErrorRecord,
  NodeInventory,
  NodeState,
  Tone
} from "../types";

export interface OverviewKpi {
  label: string;
  value: string;
  unit: string;
  detail: string;
  detailTone: Tone;
  trend: number[];
}

export type AttentionTarget =
  | {kind: "load-error"; id: string}
  | {kind: "load-capture"; nodeID: string; attemptID: string}
  | {kind: "node"; nodeID: string}
  | {kind: "analytics"};

export interface AttentionItem {
  key: string;
  tone: Tone;
  title: string;
  detail: string;
  actionLabel: string;
  target: AttentionTarget;
}

export interface AttentionSources {
  nodes: readonly NodeInventory[];
  snapshots: Readonly<Record<string, NodeState>>;
  loadErrors: readonly LoadErrorRecord[];
  loadErrorsEnabled: boolean;
  failedCaptures: readonly LoadCaptureAttempt[];
  analyticsNodeErrors: readonly {node_id?: string; node_url?: string; error: string}[];
}

const attentionLimit = 8;
const recentRequestLimit = 12;

export function overviewKpis(analytics: AnalyticsResponse | null, snapshots: readonly NodeState[], loadErrorCount: number): OverviewKpi[] {
  const summary = analytics?.enabled ? analytics.summary : null;
  const trend = (value: (bucket: AnalyticsTimeline) => number): number[] => analytics?.enabled ? bucketTrend(analytics, value) : [];
  return [
    {
      label: "Requests",
      value: summary ? formatCount(summary.request_count) : "—",
      unit: "",
      detail: summary ? `${successRate(summary.request_count, summary.success_count)} succeeded` : "Analytics is off",
      detailTone: "neutral",
      trend: trend(bucket => bucket.request_count)
    },
    {
      label: "Throughput",
      value: summary ? formatDecimal(summary.average_tokens_per_second, 1) : "—",
      unit: summary ? "tok/s" : "",
      detail: summary ? `${promptSpeedPrefix(summary.average_prompt_tokens_per_second)}${formatDecimal(summary.average_duration_ms / 1000, 1)} s per request` : "Analytics is off",
      detailTone: "neutral",
      trend: trend(bucket => bucket.average_tokens_per_second ?? 0)
    },
    memoryKpi(snapshots, trend(bucket => bucket.vram_peak_mb)),
    {
      label: "Failures",
      value: summary ? formatCount(summary.failure_count) : "—",
      unit: "",
      detail: loadErrorCount > 0 ? `${formatCount(loadErrorCount)} load error${loadErrorCount === 1 ? "" : "s"}` : "No load errors",
      detailTone: loadErrorCount > 0 || (summary?.failure_count ?? 0) > 0 ? "danger" : "success",
      trend: trend(bucket => bucket.failure_count ?? 0)
    }
  ];
}

function bucketTrend(analytics: AnalyticsResponse, value: (bucket: AnalyticsTimeline) => number): number[] {
  const bucketsByStart = new Map(analytics.timeline.map(bucket => [bucket.bucket_start, bucket]));
  return filledBucketStarts(analytics.timeline, {from: analytics.from, to: analytics.to, granularity: analytics.granularity})
    .map(bucketStart => {
      const bucket = bucketsByStart.get(bucketStart);
      return bucket ? value(bucket) : 0;
    });
}

function successRate(requests: number, successes: number): string {
  if (requests <= 0) {
    return "none";
  }
  return `${formatDecimal((successes / requests) * 100, 1)}%`;
}

function memoryKpi(snapshots: readonly NodeState[], trend: number[]): OverviewKpi {
  const readings = snapshots.flatMap(snapshot => snapshot.memory ? [snapshot.memory] : []);
  if (readings.length === 0) {
    return {label: "Memory in use", value: "—", unit: "", detail: "No node reports memory", detailTone: "neutral", trend};
  }
  const kinds = new Set(readings.map(reading => reading.kind));
  const label = kinds.size > 1 ? "Memory in use" : kinds.has("vram") ? "VRAM in use" : "RAM in use";
  const used = readings.reduce((sum, reading) => sum + reading.used_mb, 0);
  const total = readings.reduce((sum, reading) => sum + reading.total_mb, 0);
  const lent = snapshots
    .flatMap(snapshot => snapshot.backends.flatMap(backend => backend.loaded_models))
    .filter(row => row.borrowed)
    .reduce((sum, row) => sum + (row.memory_estimate_mb ?? 0), 0);
  return {
    label,
    value: gigabyteFigure(used),
    unit: `/ ${formatGigabytes(total)}`,
    detail: lent > 0 ? `${formatGigabytes(lent)} running lent work` : `${readings.length} node${readings.length === 1 ? "" : "s"} reporting`,
    detailTone: "neutral",
    trend
  };
}

export function attentionItems(sources: AttentionSources): AttentionItem[] {
  return [
    ...loadErrorItems(sources),
    ...captureItems(sources.failedCaptures),
    ...nodeHealthItems(sources.nodes),
    ...backendLifecycleItems(sources.nodes, sources.snapshots),
    ...sources.analyticsNodeErrors.map((error, index): AttentionItem => ({
      key: `analytics-${index}`,
      tone: "warning",
      title: `No analytics from ${error.node_id || error.node_url || "a node"}`,
      detail: error.error,
      actionLabel: "Analytics",
      target: {kind: "analytics"}
    }))
  ].slice(0, attentionLimit);
}

function loadErrorItems(sources: AttentionSources): AttentionItem[] {
  if (!sources.loadErrorsEnabled) {
    return [];
  }
  return [...sources.loadErrors]
    .sort((left, right) => severityRank(left.severity) - severityRank(right.severity) || right.last_seen_at.localeCompare(left.last_seen_at))
    .slice(0, 3)
    .map(record => ({
      key: `load-error-${record.id}`,
      tone: record.severity === "error" ? "danger" : "warning",
      title: `${record.model_id || record.config_name || record.phase} ${record.severity === "error" ? "failed" : "warned"}${record.node_id ? ` on ${record.node_id}` : ""}`,
      detail: `${record.message}${record.occurrences > 1 ? ` · ${formatCount(record.occurrences)}×` : ""}`,
      actionLabel: "Inspect",
      target: {kind: "load-error", id: record.id}
    }));
}

function severityRank(severity: string): number {
  return severity === "error" ? 0 : 1;
}

function captureItems(captures: readonly LoadCaptureAttempt[]): AttentionItem[] {
  return captures.slice(0, 2).map(attempt => ({
    key: `capture-${attempt.node_id}-${attempt.id}`,
    tone: "danger",
    title: `${attempt.runtime || attempt.backend_mode} load failed on ${attempt.node_id}`,
    detail: attempt.failure_message || attempt.failure_class || "Backend load failed",
    actionLabel: "Capture",
    target: {kind: "load-capture", nodeID: attempt.node_id, attemptID: attempt.id}
  }));
}

function nodeHealthItems(nodes: readonly NodeInventory[]): AttentionItem[] {
  const buildVersion = clusterBuildVersion(nodes);
  return nodes.flatMap((node): AttentionItem[] => {
    const items: AttentionItem[] = [];
    if (!node.available) {
      items.push({key: `down-${node.node_id}`, tone: "danger", title: `${node.node_id} is down`, detail: node.node_url || "No response from the node", actionLabel: "Node", target: {kind: "node", nodeID: node.node_id}});
    }
    if (node.error) {
      items.push({key: `scan-${node.node_id}`, tone: "danger", title: `${node.node_id} scan failed`, detail: node.error, actionLabel: "Node", target: {kind: "node", nodeID: node.node_id}});
    }
    if (buildVersion !== "" && node.build_version !== buildVersion) {
      items.push({
        key: `build-${node.node_id}`,
        tone: "warning",
        title: `${node.node_id} runs ${node.build_version || "an unknown build"}`,
        detail: `The master runs ${buildVersion}`,
        actionLabel: "Node",
        target: {kind: "node", nodeID: node.node_id}
      });
    }
    return items;
  });
}

function backendLifecycleItems(nodes: readonly NodeInventory[], snapshots: Readonly<Record<string, NodeState>>): AttentionItem[] {
  return nodes.flatMap(node => (snapshots[node.node_id]?.backends ?? [])
    .filter(backend => backend.lifecycle_state === "failed" || backend.lifecycle_state === "needs_init")
    .map((backend): AttentionItem => ({
      key: `backend-${node.node_id}-${backend.id}`,
      tone: backend.lifecycle_state === "failed" ? "danger" : "warning",
      title: `${backend.display_name} on ${node.node_id} ${backend.lifecycle_state === "failed" ? "failed" : "needs init"}`,
      detail: backend.error || "Initialize the backend before it can serve models",
      actionLabel: "Node",
      target: {kind: "node", nodeID: node.node_id}
    })));
}

export function recentRequests(analytics: AnalyticsResponse | null): AnalyticsRecentEvent[] {
  if (!analytics?.enabled) {
    return [];
  }
  return analytics.recent.filter(event => event.event_type === "request").slice(0, recentRequestLimit);
}
