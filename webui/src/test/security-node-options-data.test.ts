import { describe, expect, it } from "vitest";
import { securityNodeOptions, selectableSecurityNodeID, syncReportedAPICertificate } from "../features/security/security-node-options-data";
import type { InventoryResponse, NodeInventory, SecurityApiCertificateState, SecuritySummary } from "../types";
import { testInventory, testNode } from "./factories";

const now = new Date("2026-06-01T00:00:00Z");

function node(nodeID: string, role: string, apiCertificate?: SecurityApiCertificateState): NodeInventory {
  const base = {...testNode(), node_id: nodeID, role};
  return apiCertificate ? {...base, api_certificate: apiCertificate} : base;
}

function inventory(role: string, nodes: NodeInventory[]): InventoryResponse {
  return {...testInventory([], []), role, nodes};
}

describe("security node options", () => {
  it("offers no node choice unless this router is a master with slaves", () => {
    expect(securityNodeOptions(null, now)).toEqual([]);
    expect(securityNodeOptions(inventory("slave", [node("slave-a", "slave")]), now)).toEqual([]);
    expect(securityNodeOptions(inventory("master", [node("master", "master")]), now)).toEqual([]);
  });

  it("lists this router first, then only slaves, flagging API certificate problems", () => {
    const options = securityNodeOptions(inventory("master", [
      node("master", "master", {present: false}),
      node("missing", "slave", {present: false}),
      node("expiring", "slave", {present: true, not_after: "2026-06-11T00:00:00Z"}),
      node("healthy", "slave", {present: true, not_after: "2027-06-01T00:00:00Z"}),
      node("unreported", "slave")
    ]), now);

    expect(options).toEqual([
      {nodeID: "", label: "This router", needsAttention: false},
      {nodeID: "missing", label: "missing (No API certificate)", needsAttention: true},
      {nodeID: "expiring", label: "expiring (API certificate expires in 10 d)", needsAttention: true},
      {nodeID: "healthy", label: "healthy", needsAttention: false},
      {nodeID: "unreported", label: "unreported", needsAttention: false}
    ]);
  });

  it("falls back to this router when the selected slave left the cluster", () => {
    const options = securityNodeOptions(inventory("master", [node("slave-a", "slave")]), now);
    expect(selectableSecurityNodeID(options, "slave-a")).toBe("slave-a");
    expect(selectableSecurityNodeID(options, "slave-gone")).toBe("");
    expect(selectableSecurityNodeID([], "slave-a")).toBe("");
  });

  it("clears a slave's warning once its own summary reports a fresh API certificate", () => {
    const current = inventory("master", [node("slave-a", "slave", {present: false}), node("slave-b", "slave", {present: false})]);
    const issued: SecurityApiCertificateState = {present: true, not_after: "2028-06-01T00:00:00Z"};
    const summary = {node_id: "slave-a", api_certificate: issued} as SecuritySummary;

    syncReportedAPICertificate(current, "slave-a", summary);
    syncReportedAPICertificate(current, "slave-b", summary);

    expect(securityNodeOptions(current, now).map(option => option.label)).toEqual(["This router", "slave-a", "slave-b (No API certificate)"]);
  });
});
