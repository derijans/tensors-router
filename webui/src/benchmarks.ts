import { getBenchmarkRecord, runBenchmark } from "./api";
import { benchmarkSections } from "./benchmark-data";
import { allNodeModels } from "./data";
import { elements } from "./elements";
import { state } from "./state";
import type { BenchmarkRecord, BenchmarkSection, BenchmarkSummary, Model } from "./types";
import { escapeAttribute, escapeHTML } from "./utils";

export function renderBenchmarks(): void {
  ensureBenchmarkSelection();
  elements.benchmarkModelSelect.innerHTML = benchmarkModels().map(model => `
    <option value="${escapeAttribute(modelKey(model))}" ${modelKey(model) === state.benchmark.modelKey ? "selected" : ""}>
      ${escapeHTML(modelLabel(model))}
    </option>
  `).join("");
  elements.benchmarkTypeSelect.value = state.benchmark.type;
  elements.benchmarkAllSections.checked = selectedAllSections();
  elements.benchmarkSections.innerHTML = benchmarkSections.map(section => `
    <label class="toggle-row">
      <input type="checkbox" value="${escapeAttribute(section)}" data-operation-group="benchmark" data-benchmark-section="${escapeAttribute(section)}" ${state.benchmark.sections.includes(section) ? "checked" : ""} ${state.benchmark.type === "general" || selectedAllSections() ? "disabled" : ""}>
      <span>${escapeHTML(section)}</span>
    </label>
  `).join("");
  elements.runBenchmarkButton.disabled = state.benchmark.running || !selectedModel();
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
    state.benchmark.error = error instanceof Error ? error.message : String(error);
  } finally {
    state.benchmark.running = false;
    renderBenchmarks();
  }
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
  const values = Array.from(elements.benchmarkSections.querySelectorAll("[data-benchmark-section]"))
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
    elements.benchmarkLatest.innerHTML = `<div class="error-text">${escapeHTML(state.benchmark.error)}</div>`;
    return;
  }
  if (!latest) {
    elements.benchmarkLatest.innerHTML = `<div class="detail-empty">No benchmark data</div>`;
    return;
  }
  const sections = benchmarkSections
    .map(section => record?.sections?.[section])
    .filter((summary): summary is BenchmarkSummary => Boolean(summary));
  elements.benchmarkLatest.innerHTML = [
    summaryCard("Latest", latest),
    ...sections.map(summary => summaryCard(summary.section, summary))
  ].join("");
}

function renderBenchmarkHistory(): void {
  const history = currentBenchmarkRecord()?.history ?? [];
  if (history.length === 0) {
    elements.benchmarkHistory.innerHTML = `<div class="detail-empty">No history yet</div>`;
    return;
  }
  elements.benchmarkHistory.innerHTML = history.slice().reverse().map(summary => `
    <article class="benchmark-row">
      <div>
        <strong>${escapeHTML(summary.section)} / ${escapeHTML(summary.status)}</strong>
        <div class="muted">${formatDate(summary.finished_at)} / ${summary.duration_ms || 0}ms</div>
      </div>
      <div class="change-list">${optionChanges(summary)}</div>
    </article>
  `).join("");
}

function summaryCard(title: string, summary: BenchmarkSummary): string {
  return `
    <article class="benchmark-card">
      <strong>${escapeHTML(title)}</strong>
      <div class="benchmark-status ${escapeAttribute(summary.status)}">${escapeHTML(summary.status)}</div>
      <div class="muted">${summary.duration_ms || 0}ms / ${formatDate(summary.finished_at)}</div>
      ${summary.error ? `<div class="error-text">${escapeHTML(summary.error)}</div>` : ""}
      <div class="metric-list">${(summary.metrics ?? []).map(metric => `
        <span>${escapeHTML(metric.name)}: ${escapeHTML(formatMetricValue(metric))}</span>
      `).join("")}</div>
    </article>
  `;
}

function formatMetricValue(metric: NonNullable<BenchmarkSummary["metrics"]>[number]): string {
  if (metric.duration_ms) {
    return `${metric.duration_ms}ms`;
  }
  if (metric.value !== undefined && metric.unit) {
    return `${formatNumber(metric.value)} ${metric.unit}`;
  }
  if (metric.value !== undefined) {
    return formatNumber(metric.value);
  }
  return metric.status;
}

function formatNumber(value: number): string {
  if (Number.isInteger(value)) {
    return value.toString();
  }
  return value.toFixed(2);
}

function optionChanges(summary: BenchmarkSummary): string {
  const changes = summary.option_changes ?? [];
  if (changes.length === 0) {
    return `<span class="muted">no option changes</span>`;
  }
  return changes.map(change => `
    <span class="chip amber">${escapeHTML(change.key)} ${escapeHTML(change.kind)}</span>
  `).join("");
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
  return allNodeModels();
}

function modelKey(model: Model | undefined): string {
  if (!model) {
    return "";
  }
  return `${model.node_id}\n${model.local_id}`;
}

function modelLabel(model: Model): string {
  return `${model.node_id || "node"} / ${model.local_id || model.public_id}`;
}

function selectedAllSections(): boolean {
  return state.benchmark.sections.length === benchmarkSections.length;
}

function isBenchmarkSection(value: string): value is BenchmarkSection {
  return benchmarkSections.includes(value as BenchmarkSection);
}

function formatDate(value: number): string {
  if (!value) {
    return "never";
  }
  return new Date(value).toLocaleString();
}
