import { describe, expect, it } from "vitest";
import { splitHostPort } from "../features/security-wizard/wizard-address-data";
import { clusterPlans, defaultClusterPort } from "../features/security-wizard/wizard-config-data";
import { healthStatuses, profileStatuses } from "../features/security-wizard/wizard-health-data";
import { availableName } from "../features/security-wizard/wizard-nodes-data";
import { wizardCluster, wizardReport, wizardStatus, wizardSummary } from "./security-wizard-fixtures";

const masterCluster = wizardCluster({role: "master", server_bind: "0.0.0.0:8080", public_url: "http://master.lan:8080", slave_urls: ["http://worker.lan:8080"]});
const slaveCluster = wizardCluster({role: "slave", server_bind: "192.168.1.20:8080", public_url: "http://worker.lan:8080", master_url: "http://master.lan:8080"});

describe("security wizard cluster configuration", () => {
  it("defaults the cluster port to the configured one, else the server port plus ten", () => {
    expect(defaultClusterPort(wizardCluster({server_bind: "0.0.0.0:8080"}))).toBe("8090");
    expect(defaultClusterPort(wizardCluster({server_bind: "0.0.0.0:8080", bind: "0.0.0.0:9443"}))).toBe("9443");
    expect(defaultClusterPort(wizardCluster({server_bind: "nonsense"}))).toBe("");
  });

  it("moves every peer URL to https on the new cluster ports and lists only what changes", () => {
    const plans = clusterPlans([
      {key: "", label: "master", role: "master", nodeURL: "", cluster: masterCluster},
      {key: "worker", label: "worker", role: "slave", nodeURL: "", cluster: slaveCluster}
    ], {worker: "9000"});

    expect(plans[0]?.yaml).toBe([
      "cluster:",
      "  bind: \"0.0.0.0:8090\"",
      "  public_url: \"https://master.lan:8090\"",
      "  slave_urls: [\"https://worker.lan:9000\"]"
    ].join("\n"));
    expect(plans[1]?.yaml).toBe([
      "cluster:",
      "  bind: \"192.168.1.20:9000\"",
      "  public_url: \"https://worker.lan:9000\"",
      "  master_url: \"https://master.lan:8090\""
    ].join("\n"));
    expect(plans[1]?.changes.map(change => change.setting)).toEqual(["bind", "public_url", "master_url"]);
  });

  it("reports no change for a router that already runs a matching cluster port", () => {
    const done = wizardCluster({role: "slave", server_bind: "127.0.0.1:18290", bind: "127.0.0.1:18291", public_url: "https://127.0.0.1:18291", master_url: ""});
    const [plan] = clusterPlans([{key: "slave", label: "slave", role: "slave", nodeURL: "", cluster: done}], {});
    expect(plan?.changes).toEqual([]);
    expect(plan?.problem).toBe("");
  });

  it("flags a cluster port equal to server.bind and a router without a known host", () => {
    const [samePort] = clusterPlans([{key: "", label: "master", role: "master", nodeURL: "", cluster: masterCluster}], {"": "8080"});
    expect(samePort?.problem).toContain("must differ");
    const hostless = wizardCluster({role: "master", server_bind: "0.0.0.0:8080"});
    const [noHost] = clusterPlans([{key: "", label: "master", role: "master", nodeURL: "", cluster: hostless}], {});
    expect(noHost?.problem).toContain("No host");
  });

  it("leaves a slave without a known host out of the master's slave_urls", () => {
    const hostless = wizardCluster({role: "slave", server_bind: "0.0.0.0:8080"});
    const [master] = clusterPlans([
      {key: "", label: "master", role: "master", nodeURL: "", cluster: masterCluster},
      {key: "lost", label: "lost", role: "slave", nodeURL: "", cluster: hostless}
    ], {});
    expect(master?.yaml).toContain("slave_urls: []");
  });

  it("brackets IPv6 hosts in binds and URLs", () => {
    expect(splitHostPort("[::1]:8080")).toEqual({host: "::1", port: "8080"});
    const ipv6 = wizardCluster({role: "master", server_bind: "[::1]:8080", public_url: "http://[::1]:8080"});
    const [plan] = clusterPlans([{key: "", label: "master", role: "master", nodeURL: "", cluster: ipv6}], {});
    expect(plan?.yaml).toContain("bind: \"[::1]:8090\"");
    expect(plan?.yaml).toContain("public_url: \"https://[::1]:8090\"");
  });
});

describe("security wizard profile and health", () => {
  it("separates secure, pending restart, blocked and switchable routers", () => {
    const statuses = profileStatuses([
      wizardReport("a", "slave", wizardSummary("a", {status: wizardStatus({profile: "secure"})})),
      wizardReport("b", "slave", wizardSummary("b", {status: wizardStatus({profile_restart_required: true})})),
      wizardReport("c", "slave", wizardSummary("c", {status: wizardStatus({secure_profile_blocker: "an inference token is required"})})),
      wizardReport("d", "slave", wizardSummary("d"))
    ]);
    expect(statuses.map(status => status.state)).toEqual(["done", "waiting", "blocked", "todo"]);
  });

  it("lists slaves before the master and calls a router healthy only with HTTPS everywhere and the secure profile", () => {
    const secureTLS = wizardStatus({profile: "secure", listeners: [{name: "api", bind: "127.0.0.1:8080", tls: true, restart_required: false}, {name: "cluster", bind: "127.0.0.1:8090", tls: true, restart_required: false}]});
    const statuses = healthStatuses([
      wizardReport("master", "master", wizardSummary("master", {status: wizardStatus({listeners: [{name: "cluster", bind: "127.0.0.1:8090", tls: false, restart_required: true}]})})),
      wizardReport("worker", "slave", wizardSummary("worker", {status: secureTLS}))
    ]);
    expect(statuses.map(status => [status.label, status.state])).toEqual([["worker", "done"], ["master", "waiting"]]);
    expect(statuses[1]?.detail).toContain("Cluster port 127.0.0.1:8090: restart required.");
  });
});

describe("security wizard credential names", () => {
  it("adds the first free numeric suffix, ignoring case", () => {
    expect(availableName("worker-identity", [])).toBe("worker-identity");
    expect(availableName("worker-identity", ["Worker-Identity", "worker-identity-2"])).toBe("worker-identity-3");
  });
});
