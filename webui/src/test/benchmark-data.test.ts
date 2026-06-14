import { describe, expect, it } from "vitest";
import { benchmarkCompactLabel, benchmarkSections } from "../benchmark-data";
import { testModel } from "./factories";

describe("benchmark data helpers", () => {
  it("keeps the supported section order stable", () => {
    expect(benchmarkSections).toEqual(["runtime", "llm", "embed", "image", "voice", "music"]);
  });

  it("formats compact model benchmark labels", () => {
    const model = testModel("alpha");
    model.benchmark = {
      latest: {
        run_id: "run-1",
        type: "section",
        section: "runtime",
        status: "success",
        started_at: 1,
        finished_at: 2,
        duration_ms: 42
      }
    };
    expect(benchmarkCompactLabel(model)).toBe("success 42ms");
    expect(benchmarkCompactLabel(testModel("beta"))).toBe("none");
  });
});
