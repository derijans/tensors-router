import { describe, expect, it } from "vitest";
import { changedNodeSelection, defaultNodeSelection, retainedNodeSelection } from "../model-filter-data";

describe("model node filters", () => {
  it("defaults to the local node and retains valid multi-node selections", () => {
    expect(defaultNodeSelection("node-b", ["node-a", "node-b"])).toEqual(["node-b"]);
    expect(retainedNodeSelection(["node-a", "node-b"], "node-b", ["node-a", "node-b", "node-c"])).toEqual(["node-a", "node-b"]);
  });

  it("makes All Nodes exclusive when selection changes", () => {
    expect(changedNodeSelection(["node-a", "*"], ["node-a"])).toEqual(["*"]);
    expect(changedNodeSelection(["*", "node-b"], ["*"])).toEqual(["node-b"]);
  });
});
