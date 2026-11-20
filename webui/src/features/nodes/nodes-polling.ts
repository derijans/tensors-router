import { getNodeState } from "../../api/nodes-api";
import { errorMessage } from "../../lib/errors";
import { state } from "../../state/app-state";
import type { NodeState } from "../../types";
import { invalidateAllPolling, nodeSlice, pollingCurrent } from "./node-slices";
import { renderNodeCardInPlace, renderNodePanel, renderNodesPanel } from "./nodes-render";

const pollIntervalMilliseconds = 1000;

export function setNodesTabActive(active: boolean): void {
  invalidateAllPolling();
  if (!active) {
    return;
  }
  for (const nodeID of state.nodes.expanded) {
    const current = nodeSlice(nodeID);
    if (current) {
      current.loading = current.snapshot === null;
    }
  }
  renderNodesPanel();
  for (const nodeID of state.nodes.expanded) {
    const current = nodeSlice(nodeID);
    if (current) {
      void pollNode(nodeID, current.pollGeneration);
    }
  }
}

export function stopNodeStatePolling(): void {
  invalidateAllPolling();
}

export async function pollNodeOnce(nodeID: string): Promise<void> {
  const snapshot = await getNodeState(nodeID);
  state.nodeSnapshots[nodeID] = snapshot;
  const current = nodeSlice(nodeID);
  if (current) {
    current.snapshot = snapshot;
  }
  renderNodesPanel();
}

export async function pollNode(nodeID: string, generation: number, clearErrorOnSuccess = true): Promise<void> {
  if (!pollingCurrent(nodeID, generation)) {
    return;
  }
  try {
    const snapshot = await getNodeState(nodeID);
    if (pollingCurrent(nodeID, generation)) {
      applyPolledSnapshot(nodeID, snapshot, clearErrorOnSuccess);
    }
  } catch (error) {
    if (pollingCurrent(nodeID, generation)) {
      applyPollFailure(nodeID, errorMessage(error));
    }
  } finally {
    schedulePoll(nodeID, generation);
  }
}

function applyPolledSnapshot(nodeID: string, snapshot: NodeState, clearErrorOnSuccess: boolean): void {
  const current = nodeSlice(nodeID);
  if (!current) {
    return;
  }
  const changed = JSON.stringify(current.snapshot) !== JSON.stringify(snapshot) || current.loading || (clearErrorOnSuccess && current.error !== "");
  current.snapshot = snapshot;
  state.nodeSnapshots[nodeID] = snapshot;
  current.loading = false;
  if (clearErrorOnSuccess) {
    current.error = "";
  }
  if (changed) {
    renderNodeCardInPlace(nodeID);
    renderNodePanel(nodeID);
  }
}

function applyPollFailure(nodeID: string, message: string): void {
  const current = nodeSlice(nodeID);
  if (!current) {
    return;
  }
  current.loading = false;
  current.error = message;
  renderNodePanel(nodeID);
}

function schedulePoll(nodeID: string, generation: number): void {
  const current = nodeSlice(nodeID);
  if (!current || !pollingCurrent(nodeID, generation)) {
    return;
  }
  current.pollTimer = window.setTimeout(() => {
    const latest = nodeSlice(nodeID);
    if (latest) {
      latest.pollTimer = null;
    }
    void pollNode(nodeID, generation);
  }, pollIntervalMilliseconds);
}
