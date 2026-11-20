import { html, setHTML } from "../../../lib/safe-html";
import { getBenchmarkRecord, runBenchmark } from "../../../api/insights-api";
import { benchmarkSections, isBenchmarkSection, modelKey, modelLabel } from "./benchmark-data";
import { allNodeModels } from "../../../state/inventory-selectors";
import { state } from "../../../state/app-state";
import type { BenchmarkRecord, BenchmarkSummary, Model } from "../../../types";
import { setControlUnavailable } from "../../shell/operations";
import { errorMessage } from "../../../lib/errors";
import { optionElement } from "../../../ui/select";
import { emptyCard, emptyState } from "../../../ui/empty-state";
import { errorText } from "../../../ui/text";
import { insightsElements } from "../insights-elements";
import { historyRow, sectionToggle, summaryCard } from "./benchmarks-view";

export function renderBenchmarks(): void {
  ensureBenchmarkSelection();
  setHTML(insightsElements.benchmarkModelSelect, html`${benchmarkModels().map(model => optionElement(modelKey(model), modelLabel(model), modelKey(model) === state.benchmark.modelKey))}`);
  insightsElements.benchmarkTypeSelect.value = state.benchmark.type;
  insightsElements.benchmarkAllSections.checked = selectedAllSections();
  const sectionsLocked = state.benchmark.type === "general" || selectedAllSections();
  setHTML(insightsElements.benchmarkSections, html`${benchmarkSections.map(section => sectionToggle(section, state.benchmark.sections.includes(section), sectionsLocked))}`);
  setControlUnavailable(insightsElements.runBenchmarkButton, state.benchmark.running || !selectedModel());
  renderBenchmarkLatest();
  renderBenchmarkHistory();
}

export async function loadSelectedBenchmark(): Promise<void> {
  const model = selectedModel();
  if (!model) {
    state.benchmark.record = null;
    renderBenchmarks();
    return;
  }
  state.benchmark.error = "";
  state.benchmark.record = await getBenchmarkRecord(model.node_id || "", model.local_id);
  renderBenchmarks();
}

export async function runSelectedBenchmark(): Promise<void> {
  const model = selectedModel();
  if (!model) {
    return;
  }
  state.benchmark.running = true;
  state.benchmark.error = "";
  renderBenchmarks();
  try {
    state.benchmark.record = await runBenchmark({
      node_id: model.node_id || "",
      model_id: model.local_id,
      type: state.benchmark.type,
      sections: state.benchmark.type === "general" || selectedAllSections() ? ["all"] : state.benchmark.sections,
      iterations: 1,
      timeout_seconds: 1800
    });
  } catch (error) {
    state.benchmark.error = errorMessage(error);
  } finally {
    state.benchmark.running = false;
    renderBenchmarks();
  }
}

export function selectBenchmarkConfig(model: Model): void {
  selectBenchmarkModel(modelKey(model));
}

export function selectBenchmarkModel(value: string): void {
  state.benchmark.modelKey = value;
  state.benchmark.record = null;
  renderBenchmarks();
}

export function selectBenchmarkType(value: string): void {
  state.benchmark.type = value === "section" ? "section" : "general";
  renderBenchmarks();
}

export function toggleAllBenchmarkSections(checked: boolean): void {
  state.benchmark.sections = checked ? [...benchmarkSections] : [];
  renderBenchmarks();
}

export function updateBenchmarkSections(): void {
  const values = Array.from(insightsElements.benchmarkSections.querySelectorAll("[data-benchmark-section]"))
    .filter((input): input is HTMLInputElement => input instanceof HTMLInputElement && input.checked)
    .map(input => input.value)
    .filter(isBenchmarkSection);
  state.benchmark.sections = values;
  renderBenchmarks();
}

function renderBenchmarkLatest(): void {
  const record = currentBenchmarkRecord();
  const latest = record?.latest;
  if (state.benchmark.error) {
    setHTML(insightsElements.benchmarkLatest, errorText(state.benchmark.error, "div"));
    return;
  }
  if (!latest) {
    setHTML(insightsElements.benchmarkLatest, emptyCard("No benchmark data for this config yet."));
    return;
  }
  const sections = benchmarkSections
    .map(section => record?.sections?.[section])
    .filter((summary): summary is BenchmarkSummary => Boolean(summary));
  setHTML(insightsElements.benchmarkLatest, html`${[
    summaryCard("Latest", latest),
    ...sections.map(summary => summaryCard(summary.section, summary))
  ]}`);
}

function renderBenchmarkHistory(): void {
  const history = currentBenchmarkRecord()?.history ?? [];
  if (history.length === 0) {
    setHTML(insightsElements.benchmarkHistory, emptyState("No history yet"));
    return;
  }
  setHTML(insightsElements.benchmarkHistory, html`${history.slice().reverse().map(historyRow)}`);
}

function currentBenchmarkRecord(): BenchmarkRecord | null {
  if (state.benchmark.record) {
    return state.benchmark.record;
  }
  const model = selectedModel();
  if (!model?.benchmark) {
    return null;
  }
  const record: BenchmarkRecord = {
    node_id: model.node_id,
    model_id: model.local_id,
    history: []
  };
  if (model.benchmark.latest) {
    record.latest = model.benchmark.latest;
  }
  if (model.benchmark.sections) {
    record.sections = model.benchmark.sections;
  }
  return record;
}

function ensureBenchmarkSelection(): void {
  if (state.benchmark.modelKey && selectedModel()) {
    return;
  }
  state.benchmark.modelKey = modelKey(benchmarkModels()[0]);
}

function selectedModel(): Model | null {
  return benchmarkModels().find(model => modelKey(model) === state.benchmark.modelKey) ?? null;
}

function benchmarkModels(): Model[] {
  return allNodeModels().filter(model => !model.disabled);
}

function selectedAllSections(): boolean {
  return state.benchmark.sections.length === benchmarkSections.length;
}
