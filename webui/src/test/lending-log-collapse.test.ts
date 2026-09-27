import { describe, expect, it } from "vitest";
import { collapseDecisions } from "../lending-log-collapse";
import type { LendingDecision } from "../types";

function probe(recordedAt: string, idle: number): LendingDecision {
  return {
    recorded_at: recordedAt,
    node_id: "master",
    kind: "plan",
    trigger: "tick",
    lane: "image",
    owner_node_id: "slave",
    owner_model_id: "krea",
    helper_node_id: "master",
    helper_model_id: "krea11",
    outcome: "probe",
    reason: "owner_unpriced",
    helper_idle_ms: idle,
    slots: 1
  };
}

describe("lending log collapse", () => {
  it("folds consecutive repeats into one group spanning their times and measures", () => {
    const groups = collapseDecisions([
      probe("2026-09-26T20:11:39Z", 57145),
      probe("2026-09-26T20:11:37Z", 55144),
      probe("2026-09-26T20:10:47Z", 5002)
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0]?.count).toBe(3);
    expect(groups[0]?.latest.recorded_at).toBe("2026-09-26T20:11:39Z");
    expect(groups[0]?.earliest.recorded_at).toBe("2026-09-26T20:10:47Z");
    expect(groups[0]?.ranges.helper_idle_ms).toEqual({min: 5002, max: 57145});
    expect(groups[0]?.ranges.slots).toEqual({min: 1, max: 1});
  });

  it("keeps a group's id when a new repeat arrives at the head", () => {
    const before = collapseDecisions([probe("2026-09-26T20:11:37Z", 2), probe("2026-09-26T20:11:35Z", 1)]);
    const after = collapseDecisions([probe("2026-09-26T20:11:39Z", 3), probe("2026-09-26T20:11:37Z", 2), probe("2026-09-26T20:11:35Z", 1)]);

    expect(after[0]?.id).toBe(before[0]?.id);
  });

  it("starts a new group when anything but the measures changes, even if the old one recurs", () => {
    const lent: LendingDecision = {...probe("2026-09-26T20:11:38Z", 0), node_id: "slave", kind: "dispatch", trigger: "", outcome: "lent", reason: ""};

    const groups = collapseDecisions([probe("2026-09-26T20:11:39Z", 3), lent, probe("2026-09-26T20:11:37Z", 1)]);

    expect(groups.map(group => [group.latest.outcome, group.count])).toEqual([["probe", 1], ["lent", 1], ["probe", 1]]);
    expect(new Set(groups.map(group => group.id)).size).toBe(3);
  });
});
