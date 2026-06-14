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
        duration_ms: 42,
        metrics: [{ name: "tokens_per_second", status: "success", value: 99.5, unit: "tokens/s" }]
      }
    };
    expect(benchmarkCompactLabel(model)).toBe("success 99.5 tok/s");
    expect(benchmarkCompactLabel(testModel("beta"))).toBe("none");
  });
});
