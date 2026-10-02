import { describe, expect, it } from "vitest";
import { renderLendingSummary } from "../lending-summary-view";
import { SafeHTML } from "../safe-html";

describe("lending summary view", () => {
  it("flags a node running settings other than the master's and lists live leases", () => {
    const markup = SafeHTML.render(renderLendingSummary({
      nodes: [
        {node_id: "master", build_version: "v0.7.3", settings_fingerprint: "aaa", held_requests: []},
        {node_id: "slave-1", build_version: "v0.7.2", settings_fingerprint: "bbb", held_requests: null}
      ],
      leases: [{lane: "image", owner_node_id: "slave-1", owner_model_id: "krea", helper_node_id: "master", helper_model_id: "krea11", load_helper_model: true, restore_helper_model: true, helper_slots: 1, probe: true, expires_at: "2026-09-26T20:11:30Z"}]
    }, "aaa", new Date("2026-09-26T20:11:00Z")));

    expect(markup).toContain('class="badge tone-success">in sync</span>');
    expect(markup).toContain('class="badge tone-warning">differs (bbb)</span>');
    expect(markup).toContain("slave-1/krea → master/krea11");
    expect(markup).toContain("expires in 30s");
  });

  it("renders a summary with no leases reported", () => {
    const markup = SafeHTML.render(renderLendingSummary({nodes: [], leases: null}, "", new Date()));

    expect(markup).toContain("No live leases.");
  });
});
