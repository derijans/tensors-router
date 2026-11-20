import { reportErrorToConsole } from "../../api/console-report";
import { holdsOpenPopover } from "../../lib/dom";
import { type SafeHTML, html, setHTML, setOuterHTML } from "../../lib/safe-html";
import { pluralSuffix } from "../../lib/text";
import { state } from "../../state/app-state";
import type { NodeInventory } from "../../types";
import { inventoryScanNotices } from "./inventory-scan-notices-view";
import { clusterBuildVersion, nodeCardID, nodeStatePanelID, renderNodeCard } from "./node-card-view";
import { defaultNodeSlice, nodeExpanded, nodeSlice, reconcileNodeStateSelection } from "./node-slices";
import { nodeStatePanelMarkup } from "./node-state-panel-view";
import { nodesElements } from "./nodes-elements";

const reportedNodeScanErrors = new Set<string>();

export function renderNodesPanel(): void {
  const nodes = state.inventory?.nodes ?? [];
  reconcileNodeStateSelection(nodes);
  nodesElements.nodeCount.textContent = `${nodes.length} node${pluralSuffix(nodes.length)}`;
  setHTML(nodesElements.nodesScanNotices, scanNotices(nodes));
  const buildVersion = clusterBuildVersion(nodes);
  setHTML(nodesElements.nodesGrid, html`${nodes.map(node => nodesTabCard(node, buildVersion))}`);
  setHTML(nodesElements.nodesDetail, html`${nodes.filter(node => nodeExpanded(node.node_id)).map(node => nodeStatePanel(node.node_id))}`);
}

export function renderNodeCardInPlace(nodeID: string): void {
  const nodes = state.inventory?.nodes ?? [];
  const node = nodes.find(candidate => candidate.node_id === nodeID);
  const card = document.getElementById(nodeCardID("nodes", nodeID));
  if (node && card && !holdsOpenPopover(card)) {
    setOuterHTML(card, nodesTabCard(node, clusterBuildVersion(nodes)));
  }
}

export function renderNodePanel(nodeID: string): void {
  const panel = document.getElementById(nodeStatePanelID(nodeID));
  if (panel) {
    setOuterHTML(panel, nodeStatePanel(nodeID));
  }
}

function nodeStatePanel(nodeID: string): SafeHTML {
  return nodeStatePanelMarkup(nodeID, nodeSlice(nodeID) ?? defaultNodeSlice());
}

function scanNotices(nodes: NodeInventory[]): SafeHTML {
  const failing = nodes.filter(node => node.error);
  reportNewScanErrors(failing);
  return inventoryScanNotices(failing);
}

function reportNewScanErrors(failing: NodeInventory[]): void {
  const live = new Set(failing.map(scanErrorKey));
  for (const key of reportedNodeScanErrors) {
    if (!live.has(key)) {
      reportedNodeScanErrors.delete(key);
    }
  }
  for (const node of failing) {
    const key = scanErrorKey(node);
    if (!reportedNodeScanErrors.has(key)) {
      reportedNodeScanErrors.add(key);
      reportErrorToConsole(`node scan ${node.node_id || node.node_url || "unknown"}`, new Error(node.error || "scan failed"));
    }
  }
}

function scanErrorKey(node: NodeInventory): string {
  return `${node.node_id}:${node.error}`;
}

function nodesTabCard(node: NodeInventory, buildVersion: string): SafeHTML {
  return renderNodeCard(node, {
    placement: "nodes",
    expanded: nodeExpanded(node.node_id),
    clusterBuildVersion: buildVersion,
    snapshot: nodeSlice(node.node_id)?.snapshot ?? state.nodeSnapshots[node.node_id] ?? null,
    webuis: state.webuis.data?.data ?? []
  });
}
