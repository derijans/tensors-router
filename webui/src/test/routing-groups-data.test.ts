import { describe, expect, it } from "vitest";
import {
  candidateKey,
  groupCandidatesByNode,
  membersFromSelection,
  newlySelectedDifferentWeights,
  peerCountForModel,
  routingButtonLabel,
  selectedCandidateKeys,
  splitCandidatesByWeights
} from "../routing-groups-data";
import type { RoutingGroupCandidate, RoutingGroupsResponse } from "../types";

function candidate(overrides: Partial<RoutingGroupCandidate> = {}): RoutingGroupCandidate {
  return {
    node_id: "slave-a",
    image_id: "xl-jugg-q8",
    filename: "xl-jugg-q8.kcpps",
    weights_match: true,
    selected: false,
    ...overrides
  };
}

describe("splitCandidatesByWeights", () => {
  it("separates the same checkpoint from a genuinely different one", () => {
    const {sameWeights, differentWeights} = splitCandidatesByWeights([
      candidate(),
      candidate({image_id: "flux", weights_match: false})
    ]);

    expect(sameWeights.map(item => item.image_id)).toEqual(["xl-jugg-q8"]);
    expect(differentWeights.map(item => item.image_id)).toEqual(["flux"]);
  });

  it("returns empty buckets rather than undefined for no candidates", () => {
    expect(splitCandidatesByWeights([])).toEqual({sameWeights: [], differentWeights: []});
  });
});

describe("groupCandidatesByNode", () => {
  it("groups by node and orders nodes predictably", () => {
    const groups = groupCandidatesByNode([
      candidate({node_id: "slave-b", image_id: "b1"}),
      candidate({node_id: "slave-a", image_id: "a1"}),
      candidate({node_id: "slave-a", image_id: "a2"})
    ]);

    expect(groups.map(group => group.nodeId)).toEqual(["slave-a", "slave-b"]);
    expect(groups[0]?.candidates.map(item => item.image_id)).toEqual(["a1", "a2"]);
  });
});

describe("selectedCandidateKeys", () => {
  it("seeds the selection from what is already saved", () => {
    const selected = selectedCandidateKeys([
      candidate({image_id: "a1", selected: true}),
      candidate({image_id: "a2", selected: false})
    ]);

    expect([...selected]).toEqual([candidateKey({node_id: "slave-a", image_id: "a1"})]);
  });
});

describe("routingButtonLabel", () => {
  it("reports peers rather than members, because a group of one cannot help", () => {
    expect(routingButtonLabel(0)).toBe("Routing");
    expect(routingButtonLabel(1)).toBe("Routing · 1 peer");
    expect(routingButtonLabel(3)).toBe("Routing · 3 peers");
  });
});

describe("peerCountForModel", () => {
  const response: RoutingGroupsResponse = {
    groups: [
      {
        id: "group",
        members: [
          {node_id: "master", image_id: "sdxl"},
          {node_id: "slave-a", image_id: "xl-jugg-q8"},
          {node_id: "slave-b", image_id: "jugg-v9"}
        ]
      }
    ]
  };

  it("counts every other member of the group the model belongs to", () => {
    expect(peerCountForModel(response, {node_id: "master", image_id: "sdxl"})).toBe(2);
    expect(peerCountForModel(response, {node_id: "slave-b", image_id: "jugg-v9"})).toBe(2);
  });

  it("reports none for an ungrouped model or a missing response", () => {
    expect(peerCountForModel(response, {node_id: "master", image_id: "flux"})).toBe(0);
    expect(peerCountForModel(null, {node_id: "master", image_id: "sdxl"})).toBe(0);
  });
});

describe("newlySelectedDifferentWeights", () => {
  it("asks only when a mismatched candidate is newly ticked", () => {
    const candidates = [
      candidate({image_id: "flux", weights_match: false, selected: false}),
      candidate({image_id: "same", weights_match: true, selected: false})
    ];
    const selected = new Set([
      candidateKey({node_id: "slave-a", image_id: "flux"}),
      candidateKey({node_id: "slave-a", image_id: "same"})
    ]);

    expect(newlySelectedDifferentWeights(candidates, selected).map(item => item.image_id)).toEqual(["flux"]);
  });

  it("does not re-ask about a mismatch that is already saved", () => {
    const candidates = [candidate({image_id: "flux", weights_match: false, selected: true})];
    const selected = new Set([candidateKey({node_id: "slave-a", image_id: "flux"})]);

    expect(newlySelectedDifferentWeights(candidates, selected)).toEqual([]);
  });

  it("does not ask about a mismatch that is being removed", () => {
    const candidates = [candidate({image_id: "flux", weights_match: false, selected: true})];

    expect(newlySelectedDifferentWeights(candidates, new Set())).toEqual([]);
  });
});

describe("membersFromSelection", () => {
  it("keeps only the ticked candidates", () => {
    const candidates = [candidate({image_id: "a1"}), candidate({image_id: "a2"})];
    const selected = new Set([candidateKey({node_id: "slave-a", image_id: "a2"})]);

    expect(membersFromSelection(candidates, selected)).toEqual([{node_id: "slave-a", image_id: "a2"}]);
  });

  it("returns nothing when the operator clears the group", () => {
    expect(membersFromSelection([candidate()], new Set())).toEqual([]);
  });
});
