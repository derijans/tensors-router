import { describe, expect, it } from "vitest";
import { securityResourcePath, securityTargetPath } from "../api/security-paths";
import { clusterTokenPushMessage, securityTargetFor, securityTargetSubject } from "../features/security/security-target-data";
import type { SecurityToken } from "../types";

const clusterToken: SecurityToken = {id: "t1", name: "cluster", role: "cluster", source: "created", created_at: "2026-01-01T00:00:00Z"};

describe("security target", () => {
  it("treats an empty node selection as this router", () => {
    expect(securityTargetFor("")).toEqual({scope: "local"});
    expect(securityTargetFor("slave-a")).toEqual({scope: "node", nodeID: "slave-a"});
  });

  it("routes this router to the site API and a slave through the master relay", () => {
    expect(securityTargetPath({scope: "local"})).toBe("/api/security");
    expect(securityTargetPath({scope: "node", nodeID: "slave-a"})).toBe("/api/security/nodes/slave-a");
  });

  it("escapes node and resource IDs so they cannot reach other routes", () => {
    expect(securityTargetPath({scope: "node", nodeID: "../site?x=1"})).toBe("/api/security/nodes/..%2Fsite%3Fx%3D1");
    expect(securityResourcePath("certificates", "a/b", "pem")).toBe("/certificates/a%2Fb/pem");
    expect(securityResourcePath("csr", "k1")).toBe("/csr/k1");
  });

  it("names the router a confirmation talks about", () => {
    expect(securityTargetSubject({scope: "local"})).toBe("this router");
    expect(securityTargetSubject({scope: "node", nodeID: "slave-a"})).toBe("node slave-a");
  });

  it("tells a fresh cluster token apart from one the slave already held", () => {
    expect(clusterTokenPushMessage("slave-a", {token: clusterToken, created: true})).toBe("Sent the cluster token to slave-a as cluster");
    expect(clusterTokenPushMessage("slave-a", {token: {...clusterToken, name: "cluster-2"}, created: false})).toBe("slave-a already holds the cluster token as cluster-2");
  });
});
