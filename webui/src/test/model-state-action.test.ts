import { describe, expect, it, vi } from "vitest";
import { persistModelEnabled } from "../model-state-action";

const request = {node_id: "node-a", local_id: "model-a", enabled: false};

describe("model enabled action", () => {
  it("refreshes inventory after a successful update", async () => {
    const update = vi.fn().mockResolvedValue({});
    const refresh = vi.fn().mockResolvedValue(undefined);
    const rollback = vi.fn();
    await persistModelEnabled(request, update, refresh, rollback);
    expect(update).toHaveBeenCalledWith(request);
    expect(refresh).toHaveBeenCalledOnce();
    expect(rollback).not.toHaveBeenCalled();
  });

  it("restores the switch when the update fails", async () => {
    const failure = new Error("node unavailable");
    const rollback = vi.fn();
    await expect(persistModelEnabled(request, vi.fn().mockRejectedValue(failure), vi.fn(), rollback)).rejects.toBe(failure);
    expect(rollback).toHaveBeenCalledOnce();
  });
});
