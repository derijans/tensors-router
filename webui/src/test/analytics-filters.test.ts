import { readFileSync } from "node:fs";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../elements", () => ({
  elements: new Proxy({}, {get: () => ({innerHTML: "", value: "", checked: false, textContent: "", options: []})})
}));

const { state } = await import("../state");
const { updateAnalyticsPeriod } = await import("../analytics");
const { normalizedAnalyticsQuery } = await import("../analytics-data");

const page = readFileSync(new URL("../../index.html", import.meta.url), "utf8");
const main = readFileSync(new URL("../main.ts", import.meta.url), "utf8");

describe("analytics filter controls", () => {
  beforeEach(() => {
    state.analytics.query = {period: "24h"};
    state.analytics.showDetails = false;
  });

  it("keeps the selected period in state", () => {
    updateAnalyticsPeriod("7d");
    expect(state.analytics.query.period).toBe("7d");
    expect(normalizedAnalyticsQuery(state.analytics.query).period).toBe("7d");
  });

  it("ignores a period the router would reject", () => {
    updateAnalyticsPeriod("yesterday");
    expect(state.analytics.query.period).toBe("24h");
  });

  it("applies every filter outside the droppable task so a busy group cannot discard it", () => {
    const handlers = [
      "analyticsPeriodSelect",
      "analyticsNodeSelect",
      "analyticsModelSelect",
      "analyticsSectionSelect"
    ];
    for (const handler of handlers) {
      const pattern = new RegExp(
        `elements\\.${handler}\\.addEventListener\\("change", \\(\\) => \\{\\s*update[A-Za-z]+\\(elements\\.${handler}\\.value\\);\\s*runTask\\(`
      );
      expect(main).toMatch(pattern);
    }
  });

  it("offers a detail toggle above the recent table", () => {
    expect(page).toMatch(/<input id="analyticsDetailToggle" type="checkbox">/);
    expect(page).toMatch(/<th id="analyticsRecentDetailHeader">Metadata<\/th>/);
  });
});
