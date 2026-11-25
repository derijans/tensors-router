import type { WizardNodeReport, WizardNodeRole } from "../features/security-wizard/wizard-nodes-data";
import type { SecurityCertificate, SecurityClusterSettings, SecurityStatus, SecuritySummary } from "../types";

export const wizardNow = new Date("2026-06-01T00:00:00Z");

export function wizardCertificate(overrides: Partial<SecurityCertificate>): SecurityCertificate {
  return {
    id: "id",
    name: "name",
    kind: "identity",
    subject: "CN=name",
    issuer: "CN=name",
    dns_names: [],
    ip_addresses: [],
    fingerprint: "fingerprint",
    not_before: "2026-01-01T00:00:00Z",
    not_after: "2027-01-01T00:00:00Z",
    active: false,
    source: "created",
    created_at: "2026-01-01T00:00:00Z",
    ...overrides
  };
}

export const localAuthority = wizardCertificate({id: "ca", name: "cluster-ca", kind: "authority", subject: "CN=cluster-ca", issuer: "CN=cluster-ca", fingerprint: "ca-print"});

export const pfSenseAuthority = wizardCertificate({id: "pf", name: "pfsense", kind: "trusted_authority", subject: "CN=pfsense", issuer: "CN=pfsense", fingerprint: "pf-print"});

export function wizardStatus(overrides: Partial<SecurityStatus> = {}): SecurityStatus {
  return {
    identity_active: false,
    api_certificate_active: false,
    listeners: [{name: "api", bind: "127.0.0.1:8080", tls: false, restart_required: false}],
    profile: "trusted_lan",
    profile_restart_required: false,
    ...overrides
  };
}

export function wizardCluster(overrides: Partial<SecurityClusterSettings> = {}): SecurityClusterSettings {
  return {role: "slave", server_bind: "", bind: "", public_url: "", master_url: "", slave_urls: [], ...overrides};
}

export function wizardSummary(nodeID: string, overrides: Partial<SecuritySummary> = {}): SecuritySummary {
  return {
    node_id: nodeID,
    status: wizardStatus(),
    api_certificate: {present: false},
    tokens: [],
    certificates: [],
    pending_keys: [],
    default_hosts: ["127.0.0.1"],
    cluster: wizardCluster(),
    ...overrides
  };
}

export function wizardReport(label: string, role: WizardNodeRole, summary: SecuritySummary | null, error = ""): WizardNodeReport {
  const key = role === "master" ? "" : label;
  const target = role === "master" ? {scope: "local" as const} : {scope: "node" as const, nodeID: label};
  return {node: {key, label, role, target, nodeURL: ""}, summary, error};
}
