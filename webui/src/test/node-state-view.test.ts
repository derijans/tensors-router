import { describe, expect, it } from "vitest";
import { renderNodeCard, renderNodeStateSnapshot } from "../node-state-view";
import type { BackendLifecycleState, NodeInventory, NodeState, NodeStateBackend } from "../types";

function snapshot(): NodeState {
  return {
    node_id: "node-a",
    backends: [
      {
        id: "sd-server",
        display_name: "stable-diffusion.cpp",
        mode: "llama_sdcpp",
        lifecycle_state: "ready",
        loaded_models: []
      },
      {
        id: "koboldcpp",
        display_name: "Kobold <unsafe>",
        mode: "kobold",
        lifecycle_state: "ready",
        loaded_models: [
          {model_id: "model <one>", lane: "text", runtime_id: "kobold-text", generation: 7},
          {model_id: "model two", lane: "embeddings", runtime_id: "kobold-embeddings", generation: 8}
        ]
      }
    ],
    active_requests: ["model <one>", "model <one>"]
  };
}

describe("node state view", () => {
  it("renders escaped values, fixed backend order, and duplicate active model names", () => {
    const html = renderNodeStateSnapshot(snapshot(), "");

    expect(html).toContain("Kobold &lt;unsafe&gt;");
    expect(html).toContain("model &lt;one&gt;");
    expect(html).not.toContain("model <one>");
    expect(html.indexOf("Kobold &lt;unsafe&gt;")).toBeLessThan(html.indexOf("stable-diffusion.cpp"));
    expect(html.match(/<li>model &lt;one&gt;<\/li>/g)).toHaveLength(2);
  });

  it("renders explicit empty states", () => {
    const html = renderNodeStateSnapshot({node_id: "empty", backends: [], active_requests: []}, "");

    expect(html).toContain("No backend binaries detected.");
    expect(html).toContain("No active requests.");
  });

  it("disables only the pending runtime row", () => {
    const html = renderNodeStateSnapshot(snapshot(), "koboldcpp\u0000kobold-text");
    const buttons = html.match(/<button[^>]*>/g) || [];

    expect(buttons).toHaveLength(2);
    expect(buttons[0]).toContain("disabled");
    expect(buttons[1]).not.toContain("disabled");
    expect(buttons[0]).toContain('data-generation="7"');
  });

  it("renders the exact initialization action for needs-init and retryable failures", () => {
    const needsInitialization = renderVLLMBackend("needs_init", {selected_profile: "auto"});
    const retryableFailure = renderVLLMBackend("failed", {error: "import <failed>", retryable: true});
    const terminalFailure = renderVLLMBackend("failed", {error: "unsupported build", retryable: false});

    expect(needsInitialization).toContain('data-node-backend-init');
    expect(needsInitialization).toContain('data-profile="auto"');
    expect(needsInitialization).toContain(">backend needs init</button>");
    expect(retryableFailure).toContain(">backend needs init</button>");
    expect(retryableFailure).toContain("import &lt;failed&gt;");
    expect(terminalFailure).not.toContain("backend needs init");
  });

  it("renders initialization progress, disables start, and permits cancellation", () => {
    const html = renderVLLMBackend("initializing", {
      selected_profile: "cuda",
      detected_profile: "nvidia-h100",
      initialization_phase: "smoke <test>",
      initialization_bytes: 1024,
      initialization_total_bytes: 2048
    });

    expect(html).toContain("smoke &lt;test&gt;");
    expect(html).toContain('value="1024" max="2048"');
    expect(html).toContain("1.0 KB / 2.0 KB (50%)");
    expect(html).toMatch(/<button[^>]*disabled>backend needs init<\/button>/);
    expect(html).toContain("data-node-backend-init-cancel");
  });

  it("disables initialization and cancellation controls while each action is pending", () => {
    const needsInitialization: NodeState = {
      node_id: "node-a",
      backends: [{id: "vllm", display_name: "vLLM", mode: "vllm", lifecycle_state: "needs_init", loaded_models: []}],
      active_requests: []
    };
    const initializing: NodeState = {
      node_id: "node-a",
      backends: [{id: "vllm", display_name: "vLLM", mode: "vllm", lifecycle_state: "initializing", loaded_models: []}],
      active_requests: []
    };

    const pendingStart = renderNodeStateSnapshot(needsInitialization, "", "init\u0000vllm");
    const pendingCancel = renderNodeStateSnapshot(initializing, "", "cancel\u0000vllm");

    expect(pendingStart).toMatch(/<button[^>]*disabled>backend needs init<\/button>/);
    expect(pendingCancel).toContain('data-node-backend-init-cancel data-backend-id="vllm" disabled>Cancelling...</button>');
  });

  it("renders ready identity and suppresses init for unsupported or missing companions", () => {
    const ready = renderVLLMBackend("ready", {runtime_version: "0.10.2", selected_profile: "rocm"});
    const unsupported = renderVLLMBackend("unsupported", {error: "Requires Linux <host>"});
    const missing = renderVLLMBackend("companion_missing");

    expect(ready).toContain("Version: 0.10.2");
    expect(ready).toContain("Profile: rocm");
    expect(ready).toContain("No loaded models.");
    expect(unsupported).toContain("Requires Linux &lt;host&gt;");
    expect(unsupported).not.toContain("data-node-backend-init");
    expect(missing).toContain("vLLM companion is missing.");
    expect(missing).not.toContain("data-node-backend-init");
  });
});

function renderVLLMBackend(lifecycleState: BackendLifecycleState, values: Partial<NodeStateBackend> = {}): string {
  return renderNodeStateSnapshot({
    node_id: "node-a",
    backends: [{
      id: "vllm",
      display_name: "vLLM",
      mode: "vllm",
      lifecycle_state: lifecycleState,
      loaded_models: [],
      ...values
    }],
    active_requests: []
  }, "");
}
  it("renders node selection as a keyboard-clickable button", () => {
    const node: NodeInventory = {
      node_id: "node <one>",
      source: "local",
      role: "standalone",
      backend_mode: "kobold",
      available: true,
      hardware: {max_threads: 8, gpu_backend: "cpu", gpu_count: 0},
      models: [],
      files: []
    };
    const html = renderNodeCard(node, true);

    expect(html).toMatch(/<button\b/);
    expect(html).toContain('type="button"');
    expect(html).toContain('aria-expanded="true"');
    expect(html).toContain('aria-controls="nodeStatePanel"');
    expect(html).toContain("node &lt;one&gt;");
  });
