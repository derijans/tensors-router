import { describe, expect, it } from "vitest";
import { downloadNodeStatus, enabledDownloadNodes, preferredDownloadNodeID } from "../download-capability-data";
import type { DownloadNodeCapability } from "../types";

function node(nodeID: string, enabled: boolean, working: boolean, reason = ""): DownloadNodeCapability {
  return {
    node_id: nodeID,
    available: working,
    capability: {
      enabled,
      present: working,
      working,
      available: working,
      configured: working,
      configured_token: false,
      ...(reason ? {reason, error: reason} : {})
    },
    devices: []
  };
}

describe("download capability presentation", () => {
  it("hides the Download tab data when every node explicitly disables downloading", () => {
    expect(enabledDownloadNodes([node("one", false, false), node("two", false, false)])).toEqual([]);
    expect(preferredDownloadNodeID([node("one", false, false)], "")).toBe("");
  });

  it("keeps an enabled failed node selectable and exposes its diagnostic", () => {
    const failed = node("failed", true, false, "downloader.yaml is invalid");
    expect(enabledDownloadNodes([failed])).toEqual([failed]);
    expect(preferredDownloadNodeID([failed], "")).toBe("failed");
    expect(downloadNodeStatus(failed)).toBe("downloader.yaml is invalid");
  });

  it("selects a ready node when ready and failed enabled nodes are mixed", () => {
    const ready = node("ready", true, true);
    const nodes = [node("failed", true, false, "storage inaccessible"), node("disabled", false, false), ready];
    expect(preferredDownloadNodeID(nodes, "")).toBe("ready");
    expect(enabledDownloadNodes(nodes).map(value => value.node_id)).toEqual(["failed", "ready"]);
    expect(downloadNodeStatus(ready)).toBe("ready");
  });

  it("preserves an existing enabled selection", () => {
    const nodes = [node("failed", true, false, "database failure"), node("ready", true, true)];
    expect(preferredDownloadNodeID(nodes, "failed")).toBe("failed");
  });
});
