import type { LendingDecision } from "./types";

export const lendingMeasureFields = [
  "pending_count", "backlog_count", "keep_ms", "switch_ms", "service_ms", "owner_job_ms",
  "helper_idle_ms", "slots", "lent_out", "borrowed_ahead", "wait_ms"
] as const;

export type LendingMeasureField = typeof lendingMeasureFields[number];

export interface MeasureRange {
  min: number;
  max: number;
}

export interface LendingDecisionGroup {
  id: string;
  latest: LendingDecision;
  earliest: LendingDecision;
  count: number;
  ranges: Partial<Record<LendingMeasureField, MeasureRange>>;
}

export function decisionSignature(decision: LendingDecision): string {
  return [
    decision.node_id, decision.kind, decision.trigger, decision.lane,
    decision.owner_node_id, decision.owner_model_id, decision.helper_node_id, decision.helper_model_id,
    decision.outcome, decision.reason
  ].map(value => value ?? "").join("\u0000");
}

export function collapseDecisions(newestFirst: LendingDecision[]): LendingDecisionGroup[] {
  const groups: LendingDecisionGroup[] = [];
  let previousSignature = "";
  for (const decision of newestFirst) {
    const signature = decisionSignature(decision);
    const current = groups.at(-1);
    if (current && signature === previousSignature) {
      current.earliest = decision;
      current.count += 1;
      widenRanges(current.ranges, decision);
      continue;
    }
    const group: LendingDecisionGroup = {id: "", latest: decision, earliest: decision, count: 1, ranges: {}};
    widenRanges(group.ranges, decision);
    groups.push(group);
    previousSignature = signature;
  }
  for (const group of groups) {
    group.id = groupID(group.earliest);
  }
  return groups;
}

function groupID(earliest: LendingDecision): string {
  return `${earliest.node_id}:${earliest.id ?? earliest.recorded_at}`;
}

function widenRanges(ranges: LendingDecisionGroup["ranges"], decision: LendingDecision): void {
  for (const field of lendingMeasureFields) {
    const value = decision[field] ?? 0;
    const range = ranges[field];
    ranges[field] = range ? {min: Math.min(range.min, value), max: Math.max(range.max, value)} : {min: value, max: value};
  }
}
