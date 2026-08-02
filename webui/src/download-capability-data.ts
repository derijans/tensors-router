import type { DownloadNodeCapability } from "./types";

export function enabledDownloadNodes(nodes: DownloadNodeCapability[]): DownloadNodeCapability[] {
  return nodes.filter(node => node.capability.enabled === true);
}

export function preferredDownloadNodeID(nodes: DownloadNodeCapability[], currentNodeID: string): string {
  const enabledNodes = enabledDownloadNodes(nodes);
  if (enabledNodes.some(node => node.node_id === currentNodeID)) {
    return currentNodeID;
  }
  return enabledNodes.find(node => node.capability.working === true)?.node_id || enabledNodes[0]?.node_id || "";
}

export function downloadNodeStatus(node: DownloadNodeCapability): string {
  if (node.capability.working) {
    return "ready";
  }
  return node.capability.reason || node.capability.error || "downloader initialization failed";
}
