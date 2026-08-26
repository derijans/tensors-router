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
    const html = renderNodeStateSnapshot("node-a", snapshot(), "");

    expect(html).toContain("Kobold &lt;unsafe&gt;");
    expect(html).toContain("model &lt;one&gt;");
    expect(html).not.toContain("model <one>");
    expect(html.indexOf("Kobold &lt;unsafe&gt;")).toBeLessThan(html.indexOf("stable-diffusion.cpp"));
    expect(html.match(/<li>model &lt;one&gt;<\/li>/g)).toHaveLength(2);
  });

  it("renders explicit empty states", () => {
    const html = renderNodeStateSnapshot("empty", {node_id: "empty", backends: [], active_requests: []}, "");

    expect(html).toContain("No backend binaries detected.");
    expect(html).toContain("No active requests.");
  });

  it("disables only the pending runtime row", () => {
    const html = renderNodeStateSnapshot("node-a", snapshot(), "koboldcpp kobold-text");
    const buttons = html.match(/<button[^>]*>/g) || [];

    expect(buttons).toHaveLength(2);
    expect(buttons[0]).toContain("disabled");
    expect(buttons[1]).not.toContain("disabled");
    expect(buttons[0]).toContain('data-generation="7"');
    expect(buttons[0]).toContain('data-node-id="node-a"');
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

    const pendingStart = renderNodeStateSnapshot("node-a", needsInitialization, "", "init vllm");
    const pendingCancel = renderNodeStateSnapshot("node-a", initializing, "", "cancel vllm");

    expect(pendingStart).toMatch(/<button[^>]*disabled>backend needs init<\/button>/);
    expect(pendingCancel).toContain('data-node-backend-init-cancel data-node-id="node-a" data-backend-id="vllm" disabled>Cancelling...</button>');
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

  it("offers the three offline launch options for vLLM and reflects the stored selection", () => {
    const html = renderVLLMBackend("ready", {
      launch_options: {hf_hub_offline: true, transformers_offline: false, hf_datasets_offline: true}
    });

    expect(html).toContain("Launch environment");
    expect(html).toContain("HF_HUB_OFFLINE");
    expect(html).toContain("TRANSFORMERS_OFFLINE");
    expect(html).toContain("HF_DATASETS_OFFLINE");
    expect(html).toContain("data-node-backend-launch-apply");
    expect(html).toContain('data-node-backend-launch-option="hf_hub_offline" data-node-id="node-a" data-backend-id="vllm" checked');
    expect(html).toContain('data-node-backend-launch-option="transformers_offline" data-node-id="node-a" data-backend-id="vllm">');
  });

  it("omits launch options for non-vLLM backends and when none are reported", () => {
    const withoutOptions = renderVLLMBackend("ready", {});
    const kobold = renderNodeStateSnapshot("node-a", snapshot(), "");

    expect(withoutOptions).not.toContain("Launch environment");
    expect(kobold).not.toContain("Launch environment");
  });

  it("calls out an unverified manifest distinctly from pinned trust tiers", () => {
    const tuf = renderVLLMBackend("ready", {runtime_version: "0.10.2", manifest_trust: "tuf"});
    const pinned = renderVLLMBackend("ready", {runtime_version: "0.10.2", manifest_trust: "operator-pinned"});
    const unverified = renderVLLMBackend("ready", {runtime_version: "0.10.2", manifest_trust: "unverified"});

    expect(tuf).not.toContain("Manifest trust:");
    expect(tuf).not.toContain("error-text");
    expect(pinned).toContain("Manifest trust: operator-pinned");
    expect(pinned).not.toContain("error-text");
    expect(unverified).not.toContain("Manifest trust:");
    expect(unverified).toContain("error-text");
    expect(unverified).toContain("Unverified install");
  });

  it("renders node selection as a keyboard-clickable button with role and source chips", () => {
    const node: NodeInventory = {
      node_id: "node <one>",
      source: "slave",
      role: "slave",
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
    expect(html).toContain('aria-controls="nodeStatePanel-node &lt;one&gt;"');
    expect(html).toContain("node &lt;one&gt;");
    expect(html).toContain(">slave</span>");
  });

  it("shows a node as down when unavailable", () => {
    const node: NodeInventory = {
      node_id: "node-b",
      source: "local",
      role: "standalone",
      backend_mode: "kobold",
      available: false,
      hardware: {max_threads: 8, gpu_backend: "cpu", gpu_count: 0},
      models: [],
      files: []
    };
    const html = renderNodeCard(node, false);

    expect(html).toContain(">down</span>");
    expect(html).not.toContain(">available</span>");
    expect(html).toContain('aria-expanded="false"');
  });

  it("shows the resolved ffmpeg path when the node reports one", () => {
    const html = renderNodeStateSnapshot("node-a", {
      ...snapshot(),
      ffmpeg_available: true,
      ffmpeg_path: "/usr/bin/ffmpeg"
    }, "");

    expect(html).toContain("<code>/usr/bin/ffmpeg</code>");
  });

  it("warns which features break when the node reports no ffmpeg", () => {
    const html = renderNodeStateSnapshot("node-a", {...snapshot(), ffmpeg_available: false}, "");

    expect(html).toContain("Not available.");
    expect(html).toContain("Video generation");
  });

  // A node from before ffmpeg reporting must not be shown as missing it: a
  // cluster can contain one part-way through a rolling upgrade.
  it("says nothing when the node does not report ffmpeg at all", () => {
    const html = renderNodeStateSnapshot("node-a", snapshot(), "");

    expect(html).not.toContain("ffmpeg");
  });

  it("escapes an ffmpeg path so a hostile value cannot inject markup", () => {
    const html = renderNodeStateSnapshot("node-a", {
      ...snapshot(),
      ffmpeg_available: true,
      ffmpeg_path: "/opt/<img src=x onerror=alert(1)>/ffmpeg"
    }, "");

    expect(html).not.toContain("<img src=x");
    expect(html).toContain("&lt;img src=x");
  });
});

function renderVLLMBackend(lifecycleState: BackendLifecycleState, values: Partial<NodeStateBackend> = {}): string {
  return renderNodeStateSnapshot("node-a", {
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
