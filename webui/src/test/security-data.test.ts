import { describe, expect, it } from "vitest";
import {
  expiryStatus,
  listenerNotice,
  parseHosts,
  pemFilename,
  profilePrompt,
  shortFingerprint,
  sortedCertificates,
  sortedTokens
} from "../features/security/security-data";
import type { SecurityCertificate, SecurityStatus, SecurityToken } from "../types";

function token(name: string, role: SecurityToken["role"]): SecurityToken {
  return {id: name, name, role, source: "created", created_at: "2026-01-01T00:00:00Z"};
}

function certificate(name: string, kind: SecurityCertificate["kind"], active = false): SecurityCertificate {
  return {
    id: name, name, kind, active, subject: `CN=${name}`, issuer: `CN=${name}`, dns_names: [], ip_addresses: [],
    fingerprint: "ab".repeat(32), not_before: "2026-01-01T00:00:00Z", not_after: "2028-01-01T00:00:00Z", created_at: "2026-01-01T00:00:00Z"
  };
}

function status(overrides: Partial<SecurityStatus> = {}): SecurityStatus {
  return {listener_tls: false, identity_active: false, restart_required: false, profile: "trusted_lan", profile_restart_required: false, ...overrides};
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

  it("tells the operator to restart when the listener lags the active identity", () => {
    expect(listenerNotice(status({identity_active: true, restart_required: true})).tone).toBe("warning");
    expect(listenerNotice(status({listener_tls: true, identity_active: true})).tone).toBe("success");
    expect(listenerNotice(status()).tone).toBe("info");
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
});
