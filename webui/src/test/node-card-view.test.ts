import { describe, expect, it } from "vitest";
import { renderNodeCard as renderNodeCardMarkup, type NodeCardContext } from "../node-card-view";
import { SafeHTML } from "../safe-html";
import type { NodeInventory, NodeState, WebUIEntry } from "../types";

const node: NodeInventory = {
  node_id: "node <one>",
  source: "slave",
  role: "slave",
  backend_mode: "kobold",
  available: true,
  hardware: {max_threads: 8, gpu_backend: "cuda", gpu_count: 1},
  models: [],
  files: []
};

function context(overrides: Partial<NodeCardContext> = {}): NodeCardContext {
  return {placement: "nodes", expanded: false, clusterBuildVersion: "", snapshot: null, webuis: [], ...overrides};
}

function renderNodeCard(inventory: NodeInventory, cardContext: NodeCardContext): string {
  return SafeHTML.render(renderNodeCardMarkup(inventory, cardContext));
}

function snapshotWithRuntime(): NodeState {
  return {
    node_id: "node <one>",
    active_requests: ["qwen", "qwen"],
    held_requests: [],
    memory: {kind: "vram", total_mb: 24576, used_mb: 12288, sampled_at_ms: 1},
    backends: [{
      id: "koboldcpp",
      display_name: "KoboldCpp",
      mode: "kobold",
      lifecycle_state: "ready",
      loaded_models: [{model_id: "qwen", lane: "text", runtime_id: "text-0", generation: 4, memory_estimate_mb: 8192}]
    }]
  };
}

describe("node card", () => {
  it("toggles its backend panel through a keyboard-reachable button", () => {
    const html = renderNodeCard(node, context({expanded: true}));

    expect(html).toMatch(/<button type="button" data-node-select="node &lt;one&gt;" aria-expanded="true" aria-controls="nodeStatePanel-node &lt;one&gt;">/);
    expect(html).toContain(">slave</span>");
    expect(html).toContain("Online");
  });

  it("flags a node whose build differs from the master's", () => {
    expect(renderNodeCard({...node, build_version: "v0.7.2"}, context({clusterBuildVersion: "v0.7.2"}))).toContain('class="badge tone-neutral">v0.7.2</span>');
    expect(renderNodeCard({...node, build_version: "v0.7.1"}, context({clusterBuildVersion: "v0.7.2"}))).toContain('class="badge tone-warning">v0.7.1 ≠ v0.7.2</span>');
    expect(renderNodeCard(node, context({clusterBuildVersion: "v0.7.2"}))).toContain('class="badge tone-warning">unknown build ≠ v0.7.2</span>');
  });

  it("shows a node as down when unavailable", () => {
    const html = renderNodeCard({...node, available: false}, context());

    expect(html).toContain('class="status-dot tone-danger">Down</span>');
    expect(html).not.toContain("Online");
  });

  it("splits the memory bar by loaded runtime and offers runtime actions", () => {
    const webui: WebUIEntry = {
      id: "kobold-ui",
      name: "Kobold Lite",
      backend: "koboldcpp",
      backend_mode: "kobold",
      lane: "text",
      url: "/router/webuis/kobold",
      node_id: "node <one>",
      enabled: true,
      active: true,
      active_model_id: "qwen",
      requires_loaded_model: true,
      can_open_without_model: false,
      compatible_models: []
    };
    const html = renderNodeCard(node, context({snapshot: snapshotWithRuntime(), webuis: [webui]}));

    expect(html).toContain("<b>12.0 GB / 24.0 GB</b>");
    expect(html).toContain('class="memory-segment lane-text"');
    expect(html).toContain("~8.0 GB est.");
    expect(html).toContain('class="badge tone-info">2 active</span>');
    expect(html).toContain('data-runtime-action="unload" data-node-id="node &lt;one&gt;" data-backend-id="koboldcpp" data-runtime-id="text-0" data-generation="4"');
    expect(html).toContain('data-runtime-action="webui" data-webui-id="kobold-ui"');
  });

  it("explains a node that cannot read its memory instead of drawing an empty bar", () => {
    const snapshot = snapshotWithRuntime();
    delete snapshot.memory;
    const html = renderNodeCard(node, context({snapshot}));

    expect(html).toContain("Memory unavailable on this node");
    expect(html).not.toContain("memory-bar");
  });
});
