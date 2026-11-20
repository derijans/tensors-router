import { applyNodeBackendLaunchOptions, cancelNodeBackendInitialization, initializeNodeBackend, unloadNodeRuntime } from "../../api/nodes-api";
import { errorMessage } from "../../lib/errors";
import type { BackendInitializationJob, BackendInitializationRequest, BackendLaunchOptions } from "../../types";
import { nodeBackendActionKey, nodeUnloadKey } from "./node-action-keys-data";
import { invalidatePolling, nodePanelVisible, nodeSlice, pollingCurrent } from "./node-slices";
import { applyInitializationJob } from "./nodes-data";
import { pollNode } from "./nodes-polling";
import { renderNodePanel } from "./nodes-render";

type PendingField = "pendingUnload" | "pendingBackendAction";

interface NodeAction {
  nodeID: string;
  pendingField: PendingField;
  pendingKey: string;
  perform: () => Promise<void>;
}

export async function unloadSelectedRuntime(nodeID: string, backendID: string, runtimeID: string, generation: number): Promise<void> {
  if (!backendID || !runtimeID) {
    return;
  }
  await runNodeAction({
    nodeID,
    pendingField: "pendingUnload",
    pendingKey: nodeUnloadKey(backendID, runtimeID),
    perform: async () => {
      await unloadNodeRuntime({node_id: nodeID, backend_id: backendID, runtime_id: runtimeID, expected_generation: generation});
    }
  });
}

export async function initializeSelectedBackend(nodeID: string, backendID: string, profile?: string): Promise<void> {
  await runBackendInitializationAction("init", nodeID, backendID, profile);
}

export async function cancelSelectedBackendInitialization(nodeID: string, backendID: string): Promise<void> {
  await runBackendInitializationAction("cancel", nodeID, backendID);
}

export async function applyLaunchOptionsTickedInPanel(button: HTMLButtonElement): Promise<void> {
  const nodeID = button.dataset.nodeId || "";
  const backendID = button.dataset.backendId || "";
  const panel = button.closest(".node-state-backend");
  if (!backendID || !panel) {
    return;
  }
  const options = launchOptionsTickedIn(panel);
  await runNodeAction({
    nodeID,
    pendingField: "pendingBackendAction",
    pendingKey: nodeBackendActionKey("launch-options", backendID),
    perform: async () => {
      await applyNodeBackendLaunchOptions({node_id: nodeID, backend_id: backendID, options});
    }
  });
}

function launchOptionsTickedIn(panel: Element): BackendLaunchOptions {
  const options: BackendLaunchOptions = {hf_hub_offline: false, transformers_offline: false, hf_datasets_offline: false};
  panel.querySelectorAll<HTMLInputElement>("[data-node-backend-launch-option]").forEach(checkbox => {
    const key = checkbox.dataset.nodeBackendLaunchOption as keyof BackendLaunchOptions | undefined;
    if (key && key in options) {
      options[key] = checkbox.checked;
    }
  });
  return options;
}

async function runBackendInitializationAction(action: "init" | "cancel", nodeID: string, backendID: string, profile?: string): Promise<void> {
  if (!backendID) {
    return;
  }
  const request: BackendInitializationRequest = {node_id: nodeID, backend_id: backendID};
  if (profile) {
    request.profile = profile;
  }
  await runNodeAction({
    nodeID,
    pendingField: "pendingBackendAction",
    pendingKey: nodeBackendActionKey(action, backendID),
    perform: async () => {
      const job = action === "init" ? await initializeNodeBackend(request) : await cancelNodeBackendInitialization(request);
      applyJobToSnapshot(nodeID, job);
    }
  });
}

async function runNodeAction(action: NodeAction): Promise<void> {
  const current = nodeSlice(action.nodeID);
  if (!action.nodeID || !current || current[action.pendingField]) {
    return;
  }
  invalidatePolling(action.nodeID);
  const pollGeneration = current.pollGeneration;
  current[action.pendingField] = action.pendingKey;
  current.error = "";
  renderNodePanel(action.nodeID);
  let succeeded = false;
  try {
    await action.perform();
    succeeded = true;
  } catch (error) {
    recordActionFailure(action.nodeID, pollGeneration, errorMessage(error));
  } finally {
    clearPendingAction(action);
    if (pollingCurrent(action.nodeID, pollGeneration)) {
      await pollNode(action.nodeID, pollGeneration, succeeded);
    }
  }
}

function recordActionFailure(nodeID: string, pollGeneration: number, message: string): void {
  const active = nodeSlice(nodeID);
  if (active && pollingCurrent(nodeID, pollGeneration)) {
    active.error = message;
  }
}

function clearPendingAction(action: NodeAction): void {
  const after = nodeSlice(action.nodeID);
  if (after?.[action.pendingField] !== action.pendingKey) {
    return;
  }
  after[action.pendingField] = "";
  if (nodePanelVisible(action.nodeID)) {
    renderNodePanel(action.nodeID);
  }
}

function applyJobToSnapshot(nodeID: string, job: BackendInitializationJob): void {
  const snapshot = nodeSlice(nodeID)?.snapshot;
  const backend = snapshot?.node_id === nodeID ? snapshot.backends.find(candidate => candidate.id === job.backend_id) : undefined;
  if (backend) {
    applyInitializationJob(backend, job);
  }
}
