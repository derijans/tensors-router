import { describe, expect, it } from "vitest";
import { renderNodeCard, renderNodeStateSnapshot } from "../node-state-view";
import type { NodeInventory, NodeState } from "../types";

function snapshot(): NodeState {
  return {
    node_id: "node-a",
    backends: [
      {
        id: "sd-server",
        display_name: "stable-diffusion.cpp",
        mode: "llama_sdcpp",
        loaded_models: []
      },
      {
        id: "koboldcpp",
        display_name: "Kobold <unsafe>",
        mode: "kobold",
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
});
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
