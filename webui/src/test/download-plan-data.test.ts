import { describe, expect, it } from "vitest";
import { planSelectionForMode, selectedDownloadBytes, selectedDownloadFiles, toggleDownloadPath } from "../download-plan-data";
import type { DownloadPlan } from "../types";

const plan: DownloadPlan = {
  repository: "owner/model",
  revision: "main",
  commit: "0123456789abcdef0123456789abcdef01234567",
  destination: "models/owner/model",
  total_bytes: 30,
  unsafe_warning: false,
  files: [
    {path: "model.gguf", size: 20, required: false, reason: "selected"},
    {path: "mmproj.gguf", size: 10, required: true, reason: "dependency"}
  ]
};

describe("download plan selection", () => {
  it("allows required files to be removed and recalculates selected bytes", () => {
    const selected = toggleDownloadPath(plan.files.map(file => file.path), "mmproj.gguf");
    expect(selectedDownloadFiles(plan, selected).map(file => file.path)).toEqual(["model.gguf"]);
    expect(selectedDownloadBytes(plan, selected)).toBe(20);
  });

  it("supports an empty selection so the UI can disable Start", () => {
    expect(selectedDownloadFiles(plan, [])).toEqual([]);
    expect(selectedDownloadBytes(plan, [])).toBe(0);
  });

  it("computes select all, none, and required-only selections", () => {
    expect(planSelectionForMode(plan, "all")).toEqual(["model.gguf", "mmproj.gguf"]);
    expect(planSelectionForMode(plan, "none")).toEqual([]);
    expect(planSelectionForMode(plan, "required")).toEqual(["mmproj.gguf"]);
  });
});
