import { describe, expect, it } from "vitest";
import { endpointValue, guardianRows, guardianTargets, parseEndpointValue } from "../guardian-data";
import { renderGuardianRows } from "../guardian-view";
import { SafeHTML } from "../safe-html";
import { testInventory, testModel, testNode } from "./factories";
import type { GuardianAssignment, GuardianCandidate } from "../types";

function inventory() {
  const vision = {...testModel("vision"), has_multimodal: true};
  const plain = testModel("plain");
  const embedding = {...testModel("embed"), has_llm: false, has_embeddings: true};
  return testInventory([], [], [vision, plain, embedding]);
}

describe("guardian targets", () => {
  it("offers the cluster, every node and every text-only model", () => {
    const targets = guardianTargets({...inventory(), nodes: [testNode()]});

    expect(targets.map(target => target.scope)).toEqual(["cluster", "node", "model"]);
    expect(targets[2]).toMatchObject({nodeID: "node-a", modelID: "plain"});
  });

  it("offers only the cluster before an inventory has loaded", () => {
    expect(guardianTargets(null).map(target => target.scope)).toEqual(["cluster"]);
  });
});

describe("guardian rows", () => {
  const cluster: GuardianAssignment = {scope: "cluster", guardian: {node_id: "master", model_id: "vision"}};
  const node: GuardianAssignment = {scope: "node", node_id: "node-a", guardian: {node_id: "node-a", model_id: "node-vision"}};

  it("resolves the most specific guardian for every row", () => {
    const targets = guardianTargets({...inventory(), nodes: [testNode()]});

    const rows = guardianRows(targets, [cluster, node]);

    expect(rows.map(row => row.assigned)).toEqual([cluster.guardian, node.guardian, null]);
    expect(rows.map(row => row.effective).slice(2)).toEqual([node.guardian]);
  });

  it("lets a model row fall through to the cluster guardian", () => {
    const targets = guardianTargets({...inventory(), nodes: [testNode()]});

    expect(guardianRows(targets, [cluster]).map(row => row.effective).slice(2)).toEqual([cluster.guardian]);
  });
});

describe("guardian endpoint values", () => {
  it("round-trips ids that contain separators", () => {
    const endpoint = {node_id: "node/a|b", model_id: "model \"x\""};

    expect(parseEndpointValue(endpointValue(endpoint))).toEqual(endpoint);
  });

  it("rejects values that are not an endpoint", () => {
    expect(parseEndpointValue("")).toBeNull();
    expect(parseEndpointValue("[1,2]")).toBeNull();
  });
});

describe("guardian view", () => {
  const candidates: GuardianCandidate[] = [{node_id: "master", model_id: "vision <v>", filename: "vision.kcpps"}];

  it("renders escaped candidates and marks the assigned one", () => {
    const rows = guardianRows(guardianTargets(null), [{scope: "cluster", guardian: {node_id: "master", model_id: "vision <v>"}}]);

    const html = SafeHTML.render(renderGuardianRows(rows, candidates, true));

    expect(html).toContain("master / vision &lt;v&gt;");
    expect(html).not.toContain("vision <v>");
    expect(html).toMatch(/selected>master \/ vision &lt;v&gt;<\/option>/);
  });

  it("explains when no multimodal model exists", () => {
    const html = SafeHTML.render(renderGuardianRows(guardianRows(guardianTargets(null), []), [], true));

    expect(html).toContain("No multimodal text model");
  });
});
