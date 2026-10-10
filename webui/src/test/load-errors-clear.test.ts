import { readFileSync } from "node:fs";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { LoadErrorClearResponse, LoadErrorListResponse } from "../types";

const clearLoadErrors = vi.fn();
const getLoadErrors = vi.fn();
const confirmDestructive = vi.fn();

vi.mock("../api", () => ({
  clearLoadErrors: (): Promise<LoadErrorClearResponse> => clearLoadErrors() as Promise<LoadErrorClearResponse>,
  getLoadErrors: (): Promise<LoadErrorListResponse> => getLoadErrors() as Promise<LoadErrorListResponse>
}));

vi.mock("../dialogs", () => ({
  confirmDestructive: (): Promise<boolean> => confirmDestructive() as Promise<boolean>
}));

vi.mock("../elements", () => ({
  elements: new Proxy({}, {get: () => ({innerHTML: "", value: "", options: [0, 1], append: () => undefined, hidden: false})})
}));

const page = readFileSync(new URL("../../index.html", import.meta.url), "utf8");

describe("clearing load errors", () => {
  beforeEach(() => {
    clearLoadErrors.mockReset();
    getLoadErrors.mockReset();
    confirmDestructive.mockReset();
    getLoadErrors.mockResolvedValue({enabled: true, nodes: [], records: []});
  });

  it("is offered as a Clear all button in the errors panel head", () => {
    expect(page).toMatch(/<button id="loadErrorsClearButton"[^>]*data-operation-group="load-errors"[^>]*>Clear all<\/button>/);
  });

  it("deletes nothing when the operator backs out", async () => {
    confirmDestructive.mockResolvedValue(false);
    const { clearAllLoadErrors } = await import("../load-errors");

    await clearAllLoadErrors();

    expect(clearLoadErrors).not.toHaveBeenCalled();
    expect(getLoadErrors).not.toHaveBeenCalled();
  });

  it("clears every node and reloads the list once confirmed", async () => {
    confirmDestructive.mockResolvedValue(true);
    clearLoadErrors.mockResolvedValue({cleared: 3, cleared_nodes: ["master", "slave-a"]});
    const { clearAllLoadErrors } = await import("../load-errors");

    await clearAllLoadErrors();

    expect(clearLoadErrors).toHaveBeenCalledTimes(1);
    expect(getLoadErrors).toHaveBeenCalledTimes(1);
  });

  it("reports a node that could not be cleared after refreshing the list", async () => {
    confirmDestructive.mockResolvedValue(true);
    clearLoadErrors.mockResolvedValue({cleared: 1, cleared_nodes: ["master"], node_errors: [{node_id: "slave-a", error: "unreachable"}]});
    const { clearAllLoadErrors } = await import("../load-errors");

    await expect(clearAllLoadErrors()).rejects.toThrow("slave-a: unreachable");
    expect(getLoadErrors).toHaveBeenCalledTimes(1);
  });
});
