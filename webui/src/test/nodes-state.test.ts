import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { state } from "../state";
import type { BackendInitializationJob, BackendInitializationJobState, BackendLifecycleState, NodeState } from "../types";

const apiMocks = vi.hoisted(() => ({
  cancelNodeBackendInitialization: vi.fn(),
  getNodeState: vi.fn(),
  initializeNodeBackend: vi.fn(),
  unloadNodeRuntime: vi.fn()
}));

vi.mock("../api", () => apiMocks);
vi.mock("../elements", () => ({elements: {nodeCount: {}, nodesGrid: {}}}));

import {
  cancelSelectedBackendInitialization,
  initializeSelectedBackend,
  nodeBackendActionKey,
  nodeUnloadKey,
  pollSelectedNode,
  setNodesTabActive,
  stopNodeStatePolling,
  unloadSelectedRuntime
} from "../nodes-state";

function snapshot(modelID = "model-a"): NodeState {
  return {
    node_id: "node-a",
    backends: [{
      id: "koboldcpp",
      display_name: "KoboldCpp",
      mode: "kobold",
      lifecycle_state: "ready",
      loaded_models: [{model_id: modelID, lane: "text", runtime_id: "kobold-text", generation: 4}]
    }],
    active_requests: []
  };
}

function vllmSnapshot(lifecycleState: BackendLifecycleState, nodeID = "node-a"): NodeState {
  return {
    node_id: nodeID,
    backends: [{
      id: "vllm",
      display_name: "vLLM",
      mode: "vllm",
      lifecycle_state: lifecycleState,
      loaded_models: []
    }],
    active_requests: []
  };
}

function initializationJob(stateValue: BackendInitializationJobState, values: Partial<BackendInitializationJob> = {}): BackendInitializationJob {
  return {
    job_id: "job-a",
    backend_id: "vllm",
    state: stateValue,
    selected_profile: "cuda",
    detected_profile: "nvidia",
    phase: "installing",
    completed_bytes: 10,
    total_bytes: 100,
    retryable: false,
    ...values
  };
}

function deferred<T>(): {promise: Promise<T>; resolve: (value: T) => void; reject: (error: unknown) => void} {
  let resolvePromise!: (value: T) => void;
  let rejectPromise!: (error: unknown) => void;
  return {
    promise: new Promise<T>((resolve, reject) => {
      resolvePromise = resolve;
      rejectPromise = reject;
    }),
    resolve: resolvePromise,
    reject: rejectPromise
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal("document", {getElementById: vi.fn(() => null)});
  vi.stubGlobal("window", {
    setTimeout: (callback: () => void, delay: number) => globalThis.setTimeout(callback, delay),
    clearTimeout: (timer: number) => globalThis.clearTimeout(timer)
  });
  apiMocks.getNodeState.mockReset();
  apiMocks.initializeNodeBackend.mockReset();
  apiMocks.cancelNodeBackendInitialization.mockReset();
  apiMocks.unloadNodeRuntime.mockReset();
  state.activeTab = "nodes";
  state.nodes.selectedNodeID = "node-a";
  state.nodes.snapshot = null;
  state.nodes.loading = true;
  state.nodes.error = "";
  state.nodes.pendingUnload = "";
  state.nodes.pendingBackendAction = "";
  state.nodes.pollGeneration = 1;
  state.nodes.pollTimer = null;
});

afterEach(() => {
  stopNodeStatePolling();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("node state polling", () => {
  it("fetches immediately and never overlaps requests", async () => {
    const first = deferred<NodeState>();
    apiMocks.getNodeState.mockReturnValueOnce(first.promise).mockResolvedValue(snapshot("model-b"));

    const initialPoll = pollSelectedNode(1);
    expect(apiMocks.getNodeState).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(5000);
    expect(apiMocks.getNodeState).toHaveBeenCalledTimes(1);

    first.resolve(snapshot());
    await initialPoll;
    await vi.advanceTimersByTimeAsync(999);
    expect(apiMocks.getNodeState).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(apiMocks.getNodeState).toHaveBeenCalledTimes(2);
  });

  it("stops, resumes immediately, and ignores stale responses", async () => {
    const stale = deferred<NodeState>();
    apiMocks.getNodeState.mockReturnValueOnce(stale.promise).mockResolvedValue(snapshot("fresh"));

    const stalePoll = pollSelectedNode(1);
    stopNodeStatePolling();
    stale.resolve(snapshot("stale"));
    await stalePoll;
    expect(state.nodes.snapshot).toBeNull();
    await vi.advanceTimersByTimeAsync(5000);
    expect(apiMocks.getNodeState).toHaveBeenCalledTimes(1);

    setNodesTabActive(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(apiMocks.getNodeState).toHaveBeenCalledTimes(2);
    expect(state.nodes.snapshot?.backends[0]?.loaded_models[0]?.model_id).toBe("fresh");
  });

  it("posts the exact unload target, marks only that row pending, and refreshes", async () => {
    const unload = deferred<{ok: boolean}>();
    apiMocks.unloadNodeRuntime.mockReturnValue(unload.promise);
    apiMocks.getNodeState.mockResolvedValue(snapshot("after-unload"));

    const action = unloadSelectedRuntime("koboldcpp", "kobold-text", 4);
    expect(state.nodes.pendingUnload).toBe(nodeUnloadKey("koboldcpp", "kobold-text"));
    expect(apiMocks.unloadNodeRuntime).toHaveBeenCalledWith({
      node_id: "node-a",
      backend_id: "koboldcpp",
      runtime_id: "kobold-text",
      expected_generation: 4
    });

    unload.resolve({ok: true});
    await action;
    expect(state.nodes.pendingUnload).toBe("");
    expect(apiMocks.getNodeState).toHaveBeenCalledWith("node-a");
    expect(state.nodes.snapshot?.backends[0]?.loaded_models[0]?.model_id).toBe("after-unload");
  });

  it("keeps a conflict visible while refreshing the stale row", async () => {
    apiMocks.unloadNodeRuntime.mockRejectedValue(new Error("runtime changed before unload"));
    apiMocks.getNodeState.mockResolvedValue(snapshot("replacement"));

    await unloadSelectedRuntime("koboldcpp", "kobold-text", 4);

    expect(state.nodes.error).toBe("runtime changed before unload");
    expect(state.nodes.snapshot?.backends[0]?.loaded_models[0]?.model_id).toBe("replacement");
  });

  it("starts one initialization job for duplicate requests and refreshes progress", async () => {
    const initialization = deferred<BackendInitializationJob>();
    state.nodes.snapshot = vllmSnapshot("needs_init");
    apiMocks.initializeNodeBackend.mockReturnValue(initialization.promise);
    apiMocks.getNodeState.mockResolvedValue(vllmSnapshot("initializing"));

    const first = initializeSelectedBackend("vllm", "cuda");
    const duplicate = initializeSelectedBackend("vllm", "cuda");

    expect(state.nodes.pendingBackendAction).toBe(nodeBackendActionKey("init", "vllm"));
    expect(apiMocks.initializeNodeBackend).toHaveBeenCalledTimes(1);
    expect(apiMocks.initializeNodeBackend).toHaveBeenCalledWith({node_id: "node-a", backend_id: "vllm", profile: "cuda"});
    await duplicate;

    initialization.resolve(initializationJob("running"));
    await first;

    expect(state.nodes.pendingBackendAction).toBe("");
    expect(apiMocks.getNodeState).toHaveBeenCalledWith("node-a");
    expect(state.nodes.snapshot?.backends[0]?.lifecycle_state).toBe("initializing");
  });

  it("forwards cancellation to the selected remote node", async () => {
    state.nodes.selectedNodeID = "worker";
    state.nodes.snapshot = vllmSnapshot("initializing", "worker");
    apiMocks.cancelNodeBackendInitialization.mockResolvedValue(initializationJob("cancelled"));
    apiMocks.getNodeState.mockResolvedValue(vllmSnapshot("needs_init", "worker"));

    await cancelSelectedBackendInitialization("vllm");

    expect(apiMocks.cancelNodeBackendInitialization).toHaveBeenCalledWith({node_id: "worker", backend_id: "vllm"});
    expect(apiMocks.getNodeState).toHaveBeenCalledWith("worker");
  });

  it("keeps initialization failure visible and allows retry", async () => {
    state.nodes.snapshot = vllmSnapshot("needs_init");
    const failedSnapshot = vllmSnapshot("failed");
    Object.assign(failedSnapshot.backends[0]!, {error: "missing CUDA toolkit", retryable: true});
    apiMocks.initializeNodeBackend.mockResolvedValueOnce(initializationJob("failed", {error: "missing CUDA toolkit", retryable: true}));
    apiMocks.getNodeState.mockResolvedValueOnce(failedSnapshot);

    await initializeSelectedBackend("vllm");

    expect(state.nodes.snapshot?.backends[0]?.error).toBe("missing CUDA toolkit");
    expect(state.nodes.snapshot?.backends[0]?.retryable).toBe(true);
    expect(state.nodes.pendingBackendAction).toBe("");

    apiMocks.initializeNodeBackend.mockResolvedValueOnce(initializationJob("running"));
    apiMocks.getNodeState.mockResolvedValueOnce(vllmSnapshot("initializing"));
    await initializeSelectedBackend("vllm");

    expect(apiMocks.initializeNodeBackend).toHaveBeenCalledTimes(2);
    expect(state.nodes.error).toBe("");
  });
});
