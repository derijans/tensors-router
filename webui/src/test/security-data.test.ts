import { describe, expect, it } from "vitest";
import {
  canIssue,
  canRevealSecret,
  expiryStatus,
  isRequestKind,
  listenerNotices,
  parseHosts,
  pemFilename,
  profilePrompt,
  shortFingerprint,
  sortedCertificates,
  sortedTokens,
  trustableAuthorities
} from "../features/security/security-data";
import type { SecurityCertificate, SecurityStatus, SecurityToken } from "../types";

function token(name: string, role: SecurityToken["role"]): SecurityToken {
  return {id: name, name, role, source: "created", created_at: "2026-01-01T00:00:00Z"};
}

function certificate(name: string, kind: SecurityCertificate["kind"], active = false): SecurityCertificate {
  return {
    id: name, name, kind, active, subject: `CN=${name}`, issuer: `CN=${name}`, dns_names: [], ip_addresses: [],
    fingerprint: "ab".repeat(32), not_before: "2026-01-01T00:00:00Z", not_after: "2028-01-01T00:00:00Z", source: "created", created_at: "2026-01-01T00:00:00Z"
  };
}

function status(overrides: Partial<SecurityStatus> = {}): SecurityStatus {
  return {identity_active: false, api_certificate_active: false, listeners: [], profile: "trusted_lan", profile_restart_required: false, ...overrides};
}

describe("security data", () => {
  it("orders tokens by role then name", () => {
    const names = sortedTokens([token("z", "cluster"), token("b", "admin"), token("a", "admin"), token("i", "inference")]).map(entry => entry.name);
    expect(names).toEqual(["i", "a", "b", "z"]);
  });

  it("puts the active identity first, then kinds in trust order", () => {
    const names = sortedCertificates([
      certificate("peer", "pinned_peer"),
      certificate("spare", "identity"),
      certificate("ca", "authority"),
      certificate("current", "identity", true)
    ]).map(entry => entry.name);
    expect(names).toEqual(["current", "spare", "ca", "peer"]);
  });

  it("flags expired and soon-expiring certificates", () => {
    const now = new Date("2026-06-01T00:00:00Z");
    expect(expiryStatus("2026-05-31T00:00:00Z", now).tone).toBe("danger");
    expect(expiryStatus("2026-06-11T00:00:00Z", now)).toEqual({label: "Expires in 10 d", tone: "warning"});
    expect(expiryStatus("2027-06-01T00:00:00Z", now).tone).toBe("neutral");
  });

  it("reports each listener and asks for a restart when one lags its certificates", () => {
    const notices = listenerNotices(status({
      api_certificate_active: true,
      listeners: [
        {name: "api", bind: "0.0.0.0:8080", tls: true, restart_required: false},
        {name: "cluster", bind: "0.0.0.0:8081", tls: false, restart_required: true}
      ]
    }));
    expect(notices.map(notice => notice.tone)).toEqual(["success", "warning"]);
    expect(notices[0]?.message).toContain("the API certificate");
  });

  it("warns that a shared port cannot serve the API certificate", () => {
    const notices = listenerNotices(status({api_certificate_active: true, listeners: [{name: "shared", bind: "0.0.0.0:8080", tls: true, restart_required: false}]}));
    expect(notices.at(-1)?.message).toContain("cluster.bind");
  });

  it("offers the secure profile only once certificates exist on a trusted_lan router", () => {
    expect(profilePrompt(status(), [])).toBeNull();
    expect(profilePrompt(status({profile: "secure"}), [certificate("ca", "authority")])).toBeNull();
    expect(profilePrompt(status(), [certificate("ca", "authority")])?.canSwitch).toBe(true);
  });

  it("names the blocker instead of offering a switch the router could not start with", () => {
    const prompt = profilePrompt(status({secure_profile_blocker: "an inference token is required"}), [certificate("ca", "authority")]);
    expect(prompt?.canSwitch).toBe(false);
    expect(prompt?.message).toContain("an inference token is required");
  });

  it("asks for a restart after the switch is written", () => {
    expect(profilePrompt(status({profile_restart_required: true}), [])).toEqual(expect.objectContaining({canSwitch: false}));
  });

  it("splits hosts on commas and whitespace without inventing empty entries", () => {
    expect(parseHosts(" node1.lan, 10.0.0.2\n::1 ,, ")).toEqual(["node1.lan", "10.0.0.2", "::1"]);
  });

  it("keeps file names to safe characters and preserves case", () => {
    expect(pemFilename("Node A/../x", ".crt.pem")).toBe("Node_A_.._x.crt.pem");
    expect(shortFingerprint("abcdef0123456789ffff")).toBe("abcd:ef01:2345:6789");
  });

  it("keeps secret reveal and local CA issuing to this router", () => {
    const node = {scope: "node", nodeID: "slave-a"} as const;
    expect(canRevealSecret(token("peer", "cluster"), {scope: "local"})).toBe(true);
    expect(canRevealSecret(token("peer", "cluster"), node)).toBe(false);
    expect(canRevealSecret(token("admin", "admin"), {scope: "local"})).toBe(false);
    expect(canIssue(certificate("ca", "authority"), {scope: "local"})).toBe(true);
    expect(canIssue(certificate("ca", "authority"), node)).toBe(false);
  });

  it("offers own and trusted CAs for trust, but no leaf certificates", () => {
    const names = trustableAuthorities([
      certificate("own", "authority"),
      certificate("pfsense", "trusted_authority"),
      certificate("router", "identity"),
      certificate("peer", "pinned_peer")
    ]).map(entry => entry.name);
    expect(names).toEqual(["own", "pfsense"]);
  });

  it("accepts only identity and API certificate requests", () => {
    expect(isRequestKind("identity")).toBe(true);
    expect(isRequestKind("api_certificate")).toBe(true);
    expect(isRequestKind("authority")).toBe(false);
  });
});
