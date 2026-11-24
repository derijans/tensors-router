import type { InventoryResponse, NodeInventory, SecuritySummary } from "../../types";
import { apiCertificateWarning } from "./security-api-certificate-data";

export interface SecurityNodeOption {
  nodeID: string;
  label: string;
  needsAttention: boolean;
}

const thisRouterOption: SecurityNodeOption = {nodeID: "", label: "This router", needsAttention: false};

export function securityNodeOptions(inventory: InventoryResponse | null, now: Date): SecurityNodeOption[] {
  if (inventory?.role !== "master") {
    return [];
  }
  const slaves = inventory.nodes.filter(node => node.role === "slave" && node.node_id !== "");
  if (slaves.length === 0) {
    return [];
  }
  return [thisRouterOption, ...slaves.map(slave => slaveOption(slave, now))];
}

function slaveOption(slave: NodeInventory, now: Date): SecurityNodeOption {
  const warning = apiCertificateWarning(slave.api_certificate, now);
  return {
    nodeID: slave.node_id,
    label: warning ? `${slave.node_id} (${warning.label})` : slave.node_id,
    needsAttention: warning !== null
  };
}

export function selectableSecurityNodeID(options: readonly SecurityNodeOption[], nodeID: string): string {
  return options.some(option => option.nodeID === nodeID) ? nodeID : "";
}

export function syncReportedAPICertificate(inventory: InventoryResponse | null, nodeID: string, summary: SecuritySummary | null): void {
  const node = inventory?.nodes.find(candidate => candidate.node_id === nodeID);
  if (node && summary?.node_id === nodeID) {
    node.api_certificate = summary.api_certificate;
  }
}
