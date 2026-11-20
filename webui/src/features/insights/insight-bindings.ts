import {
  flushAnalyticsToDisk,
  loadAnalytics,
  renderAnalytics,
  updateAnalyticsDetails,
  updateAnalyticsModel,
  updateAnalyticsNode,
  updateAnalyticsPeriod,
  updateAnalyticsSection
} from "./analytics/analytics";
import { refreshInventory } from "../shell/refresh";
import {
  loadSelectedBenchmark,
  runSelectedBenchmark,
  selectBenchmarkModel,
  selectBenchmarkType,
  toggleAllBenchmarkSections,
  updateBenchmarkSections
} from "./benchmarks/benchmarks";
import { elementTarget } from "../../lib/dom";
import { loadLoadCaptures, loadMoreCaptureOutput, selectLoadCapture, updateLoadCaptureFilters } from "./load-captures/load-captures";
import { downloadAnalyticsDatabases } from "./analytics/analytics-download";
import { clearAllLoadErrors, loadLoadErrors, selectLoadError } from "./load-errors/load-errors";
import { runTask } from "../shell/tasks";
import { insightsElements } from "./insights-elements";

export function bindBenchmarks(): void {
  insightsElements.benchmarkModelSelect.addEventListener("change", () => {
    selectBenchmarkModel(insightsElements.benchmarkModelSelect.value);
    runTask(loadSelectedBenchmark, "benchmark-load", "benchmark", "Loading benchmark…");
  });
  insightsElements.benchmarkTypeSelect.addEventListener("change", () => selectBenchmarkType(insightsElements.benchmarkTypeSelect.value));
  insightsElements.benchmarkAllSections.addEventListener("change", () => toggleAllBenchmarkSections(insightsElements.benchmarkAllSections.checked));
  insightsElements.benchmarkSections.addEventListener("change", updateBenchmarkSections);
  insightsElements.runBenchmarkButton.addEventListener("click", () => runTask(async () => {
    await runSelectedBenchmark();
    await refreshInventory();
  }, "benchmark-run", "benchmark", "Running benchmark…"));
}

export function bindAnalytics(): void {
  insightsElements.analyticsPeriodSelect.addEventListener("change", () => {
    updateAnalyticsPeriod(insightsElements.analyticsPeriodSelect.value);
    runTask(loadAnalytics, "analytics-period", "analytics", "Loading analytics…");
  });
  insightsElements.analyticsNodeSelect.addEventListener("change", () => {
    updateAnalyticsNode(insightsElements.analyticsNodeSelect.value);
    runTask(loadAnalytics, "analytics-node", "analytics", "Loading analytics…");
  });
  insightsElements.analyticsModelSelect.addEventListener("change", () => {
    updateAnalyticsModel(insightsElements.analyticsModelSelect.value);
    runTask(loadAnalytics, "analytics-model", "analytics", "Loading analytics…");
  });
  insightsElements.analyticsSectionSelect.addEventListener("change", () => {
    updateAnalyticsSection(insightsElements.analyticsSectionSelect.value);
    runTask(loadAnalytics, "analytics-section", "analytics", "Loading analytics…");
  });
  insightsElements.analyticsDetailToggle.addEventListener("change", () => {
    updateAnalyticsDetails(insightsElements.analyticsDetailToggle.checked);
    renderAnalytics();
  });
  insightsElements.analyticsRefreshButton.addEventListener("click", () => runTask(loadAnalytics, "analytics-refresh", "analytics", "Loading analytics…"));
  insightsElements.analyticsFlushButton.addEventListener("click", () => runTask(flushAnalyticsToDisk, "analytics-flush", "analytics", "Flushing analytics…"));
  insightsElements.analyticsDownloadButton.addEventListener("click", () => runTask(downloadAnalyticsDatabases, "analytics-download", "analytics", "Downloading database…"));
}

export function bindLoadDiagnostics(): void {
  insightsElements.loadCaptureRefreshButton.addEventListener("click", () => runTask(async () => {
    updateLoadCaptureFilters();
    await loadLoadCaptures();
  }, "load-captures-refresh", "load-captures", "Loading captures…"));
  insightsElements.loadCaptureMoreButton.addEventListener("click", () => runTask(() => loadLoadCaptures(false), "load-captures-more", "load-captures", "Loading captures…"));
  insightsElements.loadCaptureOutputMoreButton.addEventListener("click", () => runTask(loadMoreCaptureOutput, "load-capture-output", "load-captures", "Loading output…"));
  insightsElements.loadCaptureRows.addEventListener("click", event => {
    const row = elementTarget(event)?.closest<HTMLElement>("[data-load-capture-id]");
    const nodeID = row?.dataset.loadCaptureNode;
    const attemptID = row?.dataset.loadCaptureId;
    if (nodeID && attemptID) {
      runTask(() => selectLoadCapture(nodeID, attemptID), `load-capture-${attemptID}`, "load-captures", "Loading capture…");
    }
  });
  insightsElements.loadErrorsRefreshButton.addEventListener("click", () => runTask(loadLoadErrors, "load-errors-refresh", "load-errors", "Loading errors…"));
  insightsElements.loadErrorsClearButton.addEventListener("click", () => runTask(clearAllLoadErrors, "load-errors-clear", "load-errors", "Clearing errors…"));
  insightsElements.loadErrorPhaseSelect.addEventListener("change", () => runTask(loadLoadErrors, "load-errors-filter", "load-errors", "Loading errors…"));
  insightsElements.loadErrorSeveritySelect.addEventListener("change", () => runTask(loadLoadErrors, "load-errors-filter", "load-errors", "Loading errors…"));
  insightsElements.loadErrorRows.addEventListener("click", event => {
    const id = elementTarget(event)?.closest<HTMLElement>("[data-load-error-id]")?.dataset.loadErrorId;
    if (id) {
      selectLoadError(id);
    }
  });
}
