import { describe, expect, it } from "vitest";
import { preferredAuthorityID } from "../features/security-wizard/wizard-authority-data";
import { apiCertificateStatuses, identityStatuses } from "../features/security-wizard/wizard-certificate-data";
import { tokenStatuses, trustStatuses } from "../features/security-wizard/wizard-token-trust-data";
import { localAuthority, pfSenseAuthority, wizardCertificate, wizardNow, wizardReport, wizardSummary } from "./security-wizard-fixtures";

const issuedIdentity = wizardCertificate({id: "id-1", name: "worker-identity", kind: "identity", issuer: "CN=cluster-ca", subject: "CN=worker-identity", fingerprint: "leaf", active: true});

describe("security wizard authority choice", () => {
  it("keeps the chosen CA, else prefers the one that issued the master identity, else a local CA", () => {
    const certificates = [pfSenseAuthority, localAuthority, issuedIdentity];
    expect(preferredAuthorityID(certificates, "pf")).toBe("pf");
    expect(preferredAuthorityID(certificates, "")).toBe("ca");
    expect(preferredAuthorityID([pfSenseAuthority, localAuthority], "gone")).toBe("ca");
    expect(preferredAuthorityID([pfSenseAuthority], "")).toBe("pf");
    expect(preferredAuthorityID([issuedIdentity], "")).toBe("");
  });
});

describe("security wizard identity step", () => {
  it("skips a router whose active identity came from the chosen CA and plans one for the rest", () => {
    const done = wizardReport("worker", "slave", wizardSummary("worker", {certificates: [issuedIdentity]}));
    const selfSigned = wizardReport("other", "slave", wizardSummary("other", {certificates: [wizardCertificate({name: "self", active: true})]}));
    const unreachable = wizardReport("gone", "slave", null, "node request failed");

    const statuses = identityStatuses([done, selfSigned, unreachable], localAuthority, wizardNow);

    expect(statuses.map(status => status.state)).toEqual(["done", "todo", "unreachable"]);
    expect(statuses[1]?.detail).toContain("replaces self");
    expect(statuses[2]?.detail).toBe("node request failed");
  });

  it("redoes an identity from the chosen CA once it expired", () => {
    const expired = wizardCertificate({...issuedIdentity, not_after: "2026-05-01T00:00:00Z"});
    const [status] = identityStatuses([wizardReport("worker", "slave", wizardSummary("worker", {certificates: [expired]}))], localAuthority, wizardNow);
    expect(status?.state).toBe("todo");
  });

  it("waits for a signed certificate when the CA has no key on the master", () => {
    const pendingKey = {id: "pk", name: "worker-identity", kind: "identity" as const, dns_names: [], ip_addresses: [], request_pem: "CSR", created_at: "2026-05-01T00:00:00Z"};
    const report = wizardReport("worker", "slave", wizardSummary("worker", {pending_keys: [pendingKey]}));

    const [external] = identityStatuses([report], pfSenseAuthority, wizardNow);
    const [local] = identityStatuses([report], localAuthority, wizardNow);

    expect(external?.state).toBe("waiting");
    expect(external?.pendingKey?.id).toBe("pk");
    expect(local?.state).toBe("todo");
  });

  it("blocks every router until a CA is chosen", () => {
    const [status] = identityStatuses([wizardReport("worker", "slave", wizardSummary("worker"))], undefined, wizardNow);
    expect(status?.state).toBe("blocked");
  });
});

describe("security wizard API certificate step", () => {
  const migrated = wizardCertificate({name: "migrated-webui", kind: "api_certificate", source: "migrated", active: true, issuer: "CN=public-ca"});
  const keeper = wizardReport("master", "master", wizardSummary("master", {certificates: [migrated], api_certificate: {present: true, name: "migrated-webui", source: "migrated"}}));
  const bare = wizardReport("worker", "slave", wizardSummary("worker"));

  it("keeps an imported certificate unless the router is ticked, and issues where none is active", () => {
    const statuses = apiCertificateStatuses([keeper, bare], localAuthority, report => report.node.key === "worker", wizardNow);
    expect(statuses.map(status => status.state)).toEqual(["done", "todo"]);
    expect(statuses[0]?.detail).toBe("Keeps migrated-webui (migrated).");
  });

  it("replaces a kept certificate when the router is ticked, and skips an unticked router without one", () => {
    const statuses = apiCertificateStatuses([keeper, bare], localAuthority, report => report.node.key === "", wizardNow);
    expect(statuses.map(status => status.state)).toEqual(["todo", "skipped"]);
    expect(statuses[0]?.detail).toContain("replaces migrated-webui");
  });

  it("stays done after issuing even while the router remains ticked", () => {
    const issued = wizardCertificate({name: "worker-api", kind: "api_certificate", active: true, issuer: "CN=cluster-ca", fingerprint: "api-leaf"});
    const report = wizardReport("worker", "slave", wizardSummary("worker", {certificates: [issued]}));
    const [status] = apiCertificateStatuses([report], localAuthority, () => true, wizardNow);
    expect(status?.state).toBe("done");
  });
});

describe("security wizard token and trust steps", () => {
  it("creates a cluster token only on a master without one, and counts a reachable slave as holding it", () => {
    const master = wizardReport("master", "master", wizardSummary("master"));
    const slave = wizardReport("worker", "slave", wizardSummary("worker"));
    const unreachable = wizardReport("gone", "slave", null, "unauthorized");
    expect(tokenStatuses([master, slave, unreachable]).map(status => status.state)).toEqual(["todo", "done", "unreachable"]);

    const withToken = wizardReport("master", "master", wizardSummary("master", {tokens: [{id: "t", name: "mesh", role: "cluster", source: "created", created_at: ""}]}));
    expect(tokenStatuses([withToken])[0]?.state).toBe("done");
  });

  it("trusts the CA on every slave that lacks a CA with the same fingerprint", () => {
    const trusting = wizardReport("worker", "slave", wizardSummary("worker", {certificates: [wizardCertificate({...localAuthority, id: "copy", kind: "trusted_authority"})]}));
    const sameNameOtherCA = wizardReport("other", "slave", wizardSummary("other", {certificates: [wizardCertificate({kind: "trusted_authority", name: "cluster-ca", fingerprint: "different"})]}));
    const master = wizardReport("master", "master", wizardSummary("master"));
    expect(trustStatuses([master, trusting, sameNameOtherCA], localAuthority).map(status => status.state)).toEqual(["done", "done", "todo"]);
  });
});
