import {
  flushAnalyticsToDisk,
  loadAnalytics,
  renderAnalytics,
  updateAnalyticsDetails,
  updateAnalyticsModel,
  updateAnalyticsNode,
  updateAnalyticsPeriod,
  updateAnalyticsSection
} from "../analytics";
import { refreshInventory } from "../app-data";
import {
  loadSelectedBenchmark,
  runSelectedBenchmark,
  selectBenchmarkModel,
  selectBenchmarkType,
  toggleAllBenchmarkSections,
  updateBenchmarkSections
} from "../benchmarks";
import { elementTarget } from "../dom";
import { elements } from "../elements";
import { loadLoadCaptures, loadMoreCaptureOutput, selectLoadCapture, updateLoadCaptureFilters } from "../load-captures";
import { downloadAnalyticsDatabases } from "../analytics-download";
import { clearAllLoadErrors, loadLoadErrors, selectLoadError } from "../load-errors";
import { runTask } from "../tasks";

export function bindBenchmarks(): void {
  elements.benchmarkModelSelect.addEventListener("change", () => {
    selectBenchmarkModel(elements.benchmarkModelSelect.value);
    runTask(loadSelectedBenchmark, "benchmark-load", "benchmark", "Loading benchmark…");
  });
  elements.benchmarkTypeSelect.addEventListener("change", () => selectBenchmarkType(elements.benchmarkTypeSelect.value));
  elements.benchmarkAllSections.addEventListener("change", () => toggleAllBenchmarkSections(elements.benchmarkAllSections.checked));
  elements.benchmarkSections.addEventListener("change", updateBenchmarkSections);
  elements.runBenchmarkButton.addEventListener("click", () => runTask(async () => {
    await runSelectedBenchmark();
    await refreshInventory();
  }, "benchmark-run", "benchmark", "Running benchmark…"));
}

export function bindAnalytics(): void {
  elements.analyticsPeriodSelect.addEventListener("change", () => {
    updateAnalyticsPeriod(elements.analyticsPeriodSelect.value);
    runTask(loadAnalytics, "analytics-period", "analytics", "Loading analytics…");
  });
  elements.analyticsNodeSelect.addEventListener("change", () => {
    updateAnalyticsNode(elements.analyticsNodeSelect.value);
    runTask(loadAnalytics, "analytics-node", "analytics", "Loading analytics…");
  });
  elements.analyticsModelSelect.addEventListener("change", () => {
    updateAnalyticsModel(elements.analyticsModelSelect.value);
    runTask(loadAnalytics, "analytics-model", "analytics", "Loading analytics…");
  });
  elements.analyticsSectionSelect.addEventListener("change", () => {
    updateAnalyticsSection(elements.analyticsSectionSelect.value);
    runTask(loadAnalytics, "analytics-section", "analytics", "Loading analytics…");
  });
  elements.analyticsDetailToggle.addEventListener("change", () => {
    updateAnalyticsDetails(elements.analyticsDetailToggle.checked);
    renderAnalytics();
  });
  elements.analyticsRefreshButton.addEventListener("click", () => runTask(loadAnalytics, "analytics-refresh", "analytics", "Loading analytics…"));
  elements.analyticsFlushButton.addEventListener("click", () => runTask(flushAnalyticsToDisk, "analytics-flush", "analytics", "Flushing analytics…"));
  elements.analyticsDownloadButton.addEventListener("click", () => runTask(downloadAnalyticsDatabases, "analytics-download", "analytics", "Downloading database…"));
}

export function bindLoadDiagnostics(): void {
  elements.loadCaptureRefreshButton.addEventListener("click", () => runTask(async () => {
    updateLoadCaptureFilters();
    await loadLoadCaptures();
  }, "load-captures-refresh", "load-captures", "Loading captures…"));
  elements.loadCaptureMoreButton.addEventListener("click", () => runTask(() => loadLoadCaptures(false), "load-captures-more", "load-captures", "Loading captures…"));
  elements.loadCaptureOutputMoreButton.addEventListener("click", () => runTask(loadMoreCaptureOutput, "load-capture-output", "load-captures", "Loading output…"));
  elements.loadCaptureRows.addEventListener("click", event => {
    const row = elementTarget(event)?.closest<HTMLElement>("[data-load-capture-id]");
    const nodeID = row?.dataset.loadCaptureNode;
    const attemptID = row?.dataset.loadCaptureId;
    if (nodeID && attemptID) {
      runTask(() => selectLoadCapture(nodeID, attemptID), `load-capture-${attemptID}`, "load-captures", "Loading capture…");
    }
  });
  elements.loadErrorsRefreshButton.addEventListener("click", () => runTask(loadLoadErrors, "load-errors-refresh", "load-errors", "Loading errors…"));
  elements.loadErrorsClearButton.addEventListener("click", () => runTask(clearAllLoadErrors, "load-errors-clear", "load-errors", "Clearing errors…"));
  elements.loadErrorPhaseSelect.addEventListener("change", () => runTask(loadLoadErrors, "load-errors-filter", "load-errors", "Loading errors…"));
  elements.loadErrorSeveritySelect.addEventListener("change", () => runTask(loadLoadErrors, "load-errors-filter", "load-errors", "Loading errors…"));
  elements.loadErrorRows.addEventListener("click", event => {
    const id = elementTarget(event)?.closest<HTMLElement>("[data-load-error-id]")?.dataset.loadErrorId;
    if (id) {
      selectLoadError(id);
    }
  });
}
