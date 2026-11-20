import { describe, expect, it } from "vitest";
import { backendModeLabels, backendModes } from "../domain/backend-mode";

describe("backend modes", () => {
  it("offers vLLM in backend selectors", () => {
    expect(backendModes).toEqual(["kobold", "llama_sdcpp", "vllm"]);
    expect(backendModeLabels.vllm).toBe("vLLM");
  });
});
