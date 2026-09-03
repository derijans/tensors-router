import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { state } from "../state";
import type { BackendInitializationJob, BackendInitializationJobState, BackendLifecycleState, NodeInventory, NodeRuntimeSlice, NodeState } from "../types";

const apiMocks = vi.hoisted(() => ({
  cancelNodeBackendInitialization: vi.fn(),
  getNodeState: vi.fn(),
  initializeNodeBackend: vi.fn(),
  unloadNodeRuntime: vi.fn()
}));

vi.mock("../api", () => apiMocks);
vi.mock("../elements", () => ({elements: {nodeCount: {}, nodesGrid: {}, nodesDetail: {}, nodesScanNotices: {}}}));

import {
  cancelSelectedBackendInitialization,
  collapseNode,
  initializeSelectedBackend,
  nodeBackendActionKey,
  nodeUnloadKey,
  pollNode,
  reconcileNodeStateSelection,
  renderNodesPanel,
  setNodesTabActive,
  stopNodeStatePolling,
  unloadSelectedRuntime
} from "../nodes-state";
import { elements } from "../elements";

function snapshot(modelID = "model-a", nodeID = "node-a"): NodeState {
  return {
    node_id: nodeID,
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

function defaultSlice(overrides: Partial<NodeRuntimeSlice> = {}): NodeRuntimeSlice {
  return {snapshot: null, loading: true, error: "", pollGeneration: 1, pollTimer: null, pendingUnload: "", pendingBackendAction: "", ...overrides};
}

function nodeInventory(nodeID: string): NodeInventory {
  return {
    node_id: nodeID,
    node_url: `http://${nodeID}.invalid`,
    source: "local",
    role: "standalone",
    backend_mode: "kobold",
    available: true,
    hardware: {max_threads: 8, gpu_backend: "cuda", gpu_count: 1},
    models: [],
    files: []
  };
}

function expandNode(nodeID: string, overrides: Partial<NodeRuntimeSlice> = {}): void {
  if (!state.nodes.expanded.includes(nodeID)) {
    state.nodes.expanded = [...state.nodes.expanded, nodeID];
  }
  state.nodes.byNode[nodeID] = defaultSlice(overrides);
  // Keep the cached inventory in sync so renderNodesPanel's reconciliation
  // does not treat this node as vanished and collapse it right back out.
  const nodes = state.inventory?.nodes ?? [];
  if (!nodes.some(node => node.node_id === nodeID)) {
    state.inventory = {
      role: "standalone",
      node_id: "node-a",
      nodes: [...nodes, nodeInventory(nodeID)],
      models: [],
      recipes: [],
      option_catalog: [],
      observed_options: []
    };
  }
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
  state.nodes.expanded = [];
  state.nodes.byNode = {};
  expandNode("node-a");
});

afterEach(() => {
  stopNodeStatePolling();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("renderNodesPanel layout", () => {
  function orderedInventory(...nodeIDs: string[]): void {
    state.inventory = {
      role: "standalone",
      node_id: nodeIDs[0] ?? "node-a",
      nodes: nodeIDs.map(nodeInventory),
      models: [],
      recipes: [],
      option_catalog: [],
      observed_options: []
    };
  }

  it("keeps every card in the grid in inventory order and renders panels in a separate stack", () => {
    orderedInventory("node-a", "node-b", "node-c");
    state.nodes.expanded = ["node-b"];
    state.nodes.byNode = {"node-b": defaultSlice()};

    renderNodesPanel();

    const grid = (elements.nodesGrid as { innerHTML: string }).innerHTML;
    const detail = (elements.nodesDetail as { innerHTML: string }).innerHTML;

    const cardOrder = ["node-a", "node-b", "node-c"].map(id => grid.indexOf(`data-node-select="${id}"`));
    expect(cardOrder).toEqual([...cardOrder].sort((left, right) => left - right));
    expect(cardOrder.every(index => index >= 0)).toBe(true);

    expect(detail).toContain('id="nodeStatePanel-node-b"');
    expect(detail).not.toContain('id="nodeStatePanel-node-a"');
    expect(grid).not.toContain("node-state-panel");
  });

  it("does not reorder the grid when a different node is expanded", () => {
    orderedInventory("node-a", "node-b", "node-c");
    state.nodes.expanded = [];
    state.nodes.byNode = {};
    renderNodesPanel();
    const collapsedGrid = (elements.nodesGrid as { innerHTML: string }).innerHTML;

    state.nodes.expanded = ["node-a"];
    state.nodes.byNode = {"node-a": defaultSlice()};
    renderNodesPanel();
    const expandedGrid = (elements.nodesGrid as { innerHTML: string }).innerHTML;

    const order = (markup: string): number[] => ["node-a", "node-b", "node-c"].map(id => markup.indexOf(`data-node-select="${id}"`));
    expect(order(expandedGrid)).toEqual([...order(expandedGrid)].sort((left, right) => left - right));
    expect(order(collapsedGrid).map(Math.sign)).toEqual(order(expandedGrid).map(Math.sign));
  });
});

describe("node state polling", () => {
  it("fetches immediately and never overlaps requests", async () => {
    const first = deferred<NodeState>();
    apiMocks.getNodeState.mockReturnValueOnce(first.promise).mockResolvedValue(snapshot("model-b"));

    const initialPoll = pollNode("node-a", 1);
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

    const stalePoll = pollNode("node-a", 1);
    stopNodeStatePolling();
    stale.resolve(snapshot("stale"));
    await stalePoll;
    expect(state.nodes.byNode["node-a"]?.snapshot).toBeNull();
    await vi.advanceTimersByTimeAsync(5000);
    expect(apiMocks.getNodeState).toHaveBeenCalledTimes(1);

    setNodesTabActive(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(apiMocks.getNodeState).toHaveBeenCalledTimes(2);
    expect(state.nodes.byNode["node-a"]?.snapshot?.backends[0]?.loaded_models[0]?.model_id).toBe("fresh");
  });

  it("posts the exact unload target, marks only that row pending, and refreshes", async () => {
    const unload = deferred<{ok: boolean}>();
    apiMocks.unloadNodeRuntime.mockReturnValue(unload.promise);
    apiMocks.getNodeState.mockResolvedValue(snapshot("after-unload"));

    const action = unloadSelectedRuntime("node-a", "koboldcpp", "kobold-text", 4);
    expect(state.nodes.byNode["node-a"]?.pendingUnload).toBe(nodeUnloadKey("koboldcpp", "kobold-text"));
    expect(apiMocks.unloadNodeRuntime).toHaveBeenCalledWith({
      node_id: "node-a",
      backend_id: "koboldcpp",
      runtime_id: "kobold-text",
      expected_generation: 4
    });

    unload.resolve({ok: true});
    await action;
    expect(state.nodes.byNode["node-a"]?.pendingUnload).toBe("");
    expect(apiMocks.getNodeState).toHaveBeenCalledWith("node-a");
    expect(state.nodes.byNode["node-a"]?.snapshot?.backends[0]?.loaded_models[0]?.model_id).toBe("after-unload");
  });

  it("keeps a conflict visible while refreshing the stale row", async () => {
    apiMocks.unloadNodeRuntime.mockRejectedValue(new Error("runtime changed before unload"));
    apiMocks.getNodeState.mockResolvedValue(snapshot("replacement"));

    await unloadSelectedRuntime("node-a", "koboldcpp", "kobold-text", 4);

    expect(state.nodes.byNode["node-a"]?.error).toBe("runtime changed before unload");
    expect(state.nodes.byNode["node-a"]?.snapshot?.backends[0]?.loaded_models[0]?.model_id).toBe("replacement");
  });

  it("starts one initialization job for duplicate requests and refreshes progress", async () => {
    const initialization = deferred<BackendInitializationJob>();
    state.nodes.byNode["node-a"]!.snapshot = vllmSnapshot("needs_init");
    apiMocks.initializeNodeBackend.mockReturnValue(initialization.promise);
    apiMocks.getNodeState.mockResolvedValue(vllmSnapshot("initializing"));

    const first = initializeSelectedBackend("node-a", "vllm", "cuda");
    const duplicate = initializeSelectedBackend("node-a", "vllm", "cuda");

    expect(state.nodes.byNode["node-a"]?.pendingBackendAction).toBe(nodeBackendActionKey("init", "vllm"));
    expect(apiMocks.initializeNodeBackend).toHaveBeenCalledTimes(1);
    expect(apiMocks.initializeNodeBackend).toHaveBeenCalledWith({node_id: "node-a", backend_id: "vllm", profile: "cuda"});
    await duplicate;

    initialization.resolve(initializationJob("running"));
    await first;

    expect(state.nodes.byNode["node-a"]?.pendingBackendAction).toBe("");
    expect(apiMocks.getNodeState).toHaveBeenCalledWith("node-a");
    expect(state.nodes.byNode["node-a"]?.snapshot?.backends[0]?.lifecycle_state).toBe("initializing");
  });

  it("forwards actions to the target node", async () => {
    expandNode("worker", {snapshot: vllmSnapshot("initializing", "worker")});
    apiMocks.cancelNodeBackendInitialization.mockResolvedValue(initializationJob("cancelled"));
    apiMocks.getNodeState.mockResolvedValue(vllmSnapshot("needs_init", "worker"));

    await cancelSelectedBackendInitialization("worker", "vllm");

    expect(apiMocks.cancelNodeBackendInitialization).toHaveBeenCalledWith({node_id: "worker", backend_id: "vllm"});
    expect(apiMocks.getNodeState).toHaveBeenCalledWith("worker");
  });

  it("keeps initialization failure visible and allows retry", async () => {
    state.nodes.byNode["node-a"]!.snapshot = vllmSnapshot("needs_init");
    const failedSnapshot = vllmSnapshot("failed");
    Object.assign(failedSnapshot.backends[0]!, {error: "missing CUDA toolkit", retryable: true});
    apiMocks.initializeNodeBackend.mockResolvedValueOnce(initializationJob("failed", {error: "missing CUDA toolkit", retryable: true}));
    apiMocks.getNodeState.mockResolvedValueOnce(failedSnapshot);

    await initializeSelectedBackend("node-a", "vllm");

    expect(state.nodes.byNode["node-a"]?.snapshot?.backends[0]?.error).toBe("missing CUDA toolkit");
    expect(state.nodes.byNode["node-a"]?.snapshot?.backends[0]?.retryable).toBe(true);
    expect(state.nodes.byNode["node-a"]?.pendingBackendAction).toBe("");

    apiMocks.initializeNodeBackend.mockResolvedValueOnce(initializationJob("running"));
    apiMocks.getNodeState.mockResolvedValueOnce(vllmSnapshot("initializing"));
    await initializeSelectedBackend("node-a", "vllm");

    expect(apiMocks.initializeNodeBackend).toHaveBeenCalledTimes(2);
    expect(state.nodes.byNode["node-a"]?.error).toBe("");
  });
});

describe("multi-node expansion", () => {
  it("polls two expanded nodes independently", async () => {
    expandNode("node-b");
    apiMocks.getNodeState.mockImplementation((nodeID: string) => Promise.resolve(nodeID === "node-a" ? snapshot("from-a", "node-a") : snapshot("from-b", "node-b")));

    await pollNode("node-a", 1);
    await pollNode("node-b", 1);

    expect(state.nodes.byNode["node-a"]?.snapshot?.backends[0]?.loaded_models[0]?.model_id).toBe("from-a");
    expect(state.nodes.byNode["node-b"]?.snapshot?.backends[0]?.loaded_models[0]?.model_id).toBe("from-b");

    apiMocks.getNodeState.mockClear();
    await vi.advanceTimersByTimeAsync(1000);
    expect(apiMocks.getNodeState).toHaveBeenCalledWith("node-a");
    expect(apiMocks.getNodeState).toHaveBeenCalledWith("node-b");
  });

  it("an error on one node's poll does not affect the other", async () => {
    expandNode("node-b");
    apiMocks.getNodeState.mockImplementation((nodeID: string) => {
      if (nodeID === "node-a") {
        return Promise.reject(new Error("node-a unreachable"));
      }
      return Promise.resolve(snapshot("from-b", "node-b"));
    });

    await pollNode("node-a", 1);
    await pollNode("node-b", 1);

    expect(state.nodes.byNode["node-a"]?.error).toBe("node-a unreachable");
    expect(state.nodes.byNode["node-b"]?.error).toBe("");
    expect(state.nodes.byNode["node-b"]?.snapshot?.backends[0]?.loaded_models[0]?.model_id).toBe("from-b");
  });

  it("collapsing one node leaves the other's timer running", async () => {
    expandNode("node-b");
    apiMocks.getNodeState.mockResolvedValue(snapshot("steady"));

    await pollNode("node-a", 1);
    await pollNode("node-b", 1);
    apiMocks.getNodeState.mockClear();

    collapseNode("node-a");
    expect(state.nodes.expanded).not.toContain("node-a");
    expect(state.nodes.byNode["node-a"]).toBeUndefined();

    await vi.advanceTimersByTimeAsync(1000);
    expect(apiMocks.getNodeState).toHaveBeenCalledWith("node-b");
    expect(apiMocks.getNodeState).not.toHaveBeenCalledWith("node-a");
  });

  it("a pending unload on one node does not block an action on another", async () => {
    expandNode("node-b", {snapshot: snapshot("model-b", "node-b")});
    const unload = deferred<{ok: boolean}>();
    apiMocks.unloadNodeRuntime.mockReturnValue(unload.promise);
    apiMocks.getNodeState.mockResolvedValue(snapshot("after-unload"));

    const firstAction = unloadSelectedRuntime("node-a", "koboldcpp", "kobold-text", 4);
    expect(state.nodes.byNode["node-a"]?.pendingUnload).toBe(nodeUnloadKey("koboldcpp", "kobold-text"));

    const secondUnload = deferred<{ok: boolean}>();
    apiMocks.unloadNodeRuntime.mockReturnValueOnce(secondUnload.promise);
    const secondAction = unloadSelectedRuntime("node-b", "koboldcpp", "kobold-text", 4);
    expect(state.nodes.byNode["node-b"]?.pendingUnload).toBe(nodeUnloadKey("koboldcpp", "kobold-text"));

    unload.resolve({ok: true});
    secondUnload.resolve({ok: true});
    await Promise.all([firstAction, secondAction]);
  });

  it("a node disappearing from inventory stops only its timer", async () => {
    expandNode("node-b");
    apiMocks.getNodeState.mockResolvedValue(snapshot("steady"));
    await pollNode("node-a", 1);
    await pollNode("node-b", 1);
    apiMocks.getNodeState.mockClear();

    reconcileNodeStateSelection([nodeInventory("node-b")]);

    expect(state.nodes.expanded).toEqual(["node-b"]);
    expect(state.nodes.byNode["node-a"]).toBeUndefined();
    expect(state.nodes.byNode["node-b"]).toBeDefined();

    await vi.advanceTimersByTimeAsync(1000);
    expect(apiMocks.getNodeState).toHaveBeenCalledWith("node-b");
    expect(apiMocks.getNodeState).not.toHaveBeenCalledWith("node-a");
  });
});
