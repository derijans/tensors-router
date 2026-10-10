import { analyticsNodeChoices } from "./analytics-data";
import { state } from "./state";

const spacingBetweenDownloadsMs = 400;

export function databaseDownloadNodeIDs(selectedNodeID: string, liveNodeIDs: string[]): string[] {
  return selectedNodeID ? [selectedNodeID] : liveNodeIDs;
}

export function databaseDownloadPath(nodeID: string): string {
  return `/api/store/download?${new URLSearchParams({node_id: nodeID})}`;
}

export async function downloadAnalyticsDatabases(): Promise<void> {
  const liveNodeIDs = analyticsNodeChoices(state.inventory)
    .map(choice => choice.value)
    .filter(nodeID => nodeID !== "");
  const nodeIDs = databaseDownloadNodeIDs(state.analytics.query.node_id ?? "", liveNodeIDs);
  for (const [index, nodeID] of nodeIDs.entries()) {
    if (index > 0) {
      await pause(spacingBetweenDownloadsMs);
    }
    startBrowserDownload(databaseDownloadPath(nodeID));
  }
}

function startBrowserDownload(path: string): void {
  const link = document.createElement("a");
  link.href = path;
  link.download = "";
  document.body.append(link);
  link.click();
  link.remove();
}

function pause(milliseconds: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, milliseconds));
}
