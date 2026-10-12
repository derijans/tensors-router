import { describe, expect, it } from "vitest";
import { groupRequestsByModel, requestsForUnloadedModels, requestsOldestFirst, runningRequestCount } from "../node-requests-data";
import type { NodeRequest } from "../types";

const requests: NodeRequest[] = [
  {lane: "text", model_id: "qwen", state: "running", age_ms: 4000},
  {lane: "text", model_id: "qwen", state: "running", age_ms: 1000},
  {lane: "text", model_id: "qwen", state: "borrowed", age_ms: 500},
  {lane: "text", model_id: "gemma", state: "waiting_for_runtime", age_ms: 9000},
  {lane: "text", model_id: "gemma", state: "lent", age_ms: 2000, helper_node_id: "slave", helper_model_id: "gemma-alt"},
  {lane: "image", model_id: "krea", state: "queued", age_ms: 300}
];

describe("node request grouping", () => {
  it("groups requests per model with state counts, oldest first", () => {
    const groups = groupRequestsByModel(requests);

    expect(groups.map(group => group.modelID)).toEqual(["gemma", "qwen", "krea"]);
    expect(groups.map(group => group.counts.get("waiting_for_runtime"))).toEqual([1, undefined, undefined]);
    expect(groups.map(group => group.counts.get("running"))).toEqual([undefined, 2, undefined]);
    expect(groups.map(group => group.oldestAgeMS)).toEqual([9000, 4000, 300]);
    expect(groups.map(group => group.helpers)).toEqual([["slave/gemma-alt"], [], []]);
  });

  it("keeps the same model name in two lanes apart", () => {
    const groups = groupRequestsByModel([
      {lane: "text", model_id: "shared", state: "running", age_ms: 1},
      {lane: "image", model_id: "shared", state: "queued", age_ms: 1}
    ]);

    expect(groups).toHaveLength(2);
  });

  it("orders single requests oldest first without merging them", () => {
    const ordered = requestsOldestFirst(requests);

    expect(ordered).toHaveLength(requests.length);
    expect(ordered.map(request => request.age_ms)).toEqual([9000, 4000, 2000, 1000, 500, 300]);
  });

  it("counts running and borrowed requests as active for a loaded model", () => {
    expect(runningRequestCount(requests, "qwen")).toBe(3);
    expect(runningRequestCount(requests, "gemma")).toBe(0);
  });

  it("lists only the models that are not loaded", () => {
    const groups = requestsForUnloadedModels(requests, new Set(["qwen"]));

    expect(groups.map(group => group.modelID)).toEqual(["gemma", "krea"]);
  });
});
