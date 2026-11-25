import type { InventoryResponse, SecuritySummary, SecurityTarget } from "../../types";

export type WizardNodeRole = "master" | "slave";

export interface WizardNode {
  key: string;
  label: string;
  role: WizardNodeRole;
  target: SecurityTarget;
  nodeURL: string;
}

export interface WizardNodeReport {
  node: WizardNode;
  summary: SecuritySummary | null;
  error: string;
}

export function wizardNodes(inventory: InventoryResponse | null): WizardNode[] {
  if (inventory?.role !== "master") {
    return [];
  }
  const masterNode = inventory.nodes.find(node => node.role === "master");
  const master: WizardNode = {
    key: "",
    label: masterNode?.node_id || "master",
    role: "master",
    target: {scope: "local"},
    nodeURL: masterNode?.node_url ?? ""
  };
  const slaves = inventory.nodes
    .filter(node => node.role === "slave" && node.node_id !== "")
    .map((node): WizardNode => ({
      key: node.node_id,
      label: node.node_id,
      role: "slave",
      target: {scope: "node", nodeID: node.node_id},
      nodeURL: node.node_url ?? ""
    }));
  return [master, ...slaves];
}

export function masterReport(reports: readonly WizardNodeReport[]): WizardNodeReport | undefined {
  return reports.find(report => report.node.role === "master");
}

export function slaveReports(reports: readonly WizardNodeReport[]): WizardNodeReport[] {
  return reports.filter(report => report.node.role === "slave");
}

export function availableName(base: string, taken: readonly string[]): string {
  const used = new Set(taken.map(name => name.toLowerCase()));
  if (!used.has(base.toLowerCase())) {
    return base;
  }
  let suffix = 2;
  while (used.has(`${base}-${suffix}`.toLowerCase())) {
    suffix++;
  }
  return `${base}-${suffix}`;
}

export function credentialNames(summary: SecuritySummary): string[] {
  return [...summary.certificates.map(certificate => certificate.name), ...summary.pending_keys.map(key => key.name)];
}
