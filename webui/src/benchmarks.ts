import { SafeHTML, html, setHTML } from "./safe-html";
import { badge, fact } from "./markup-primitives";
import { getBenchmarkRecord, runBenchmark } from "./api";
import { benchmarkSections } from "./benchmark-data";
import { allNodeModels } from "./data";
import { elements } from "./elements";
import { state } from "./state";
import type { BenchmarkRecord, BenchmarkSection, BenchmarkSummary, Model, Tone } from "./types";
import { setControlUnavailable } from "./operations";

export function renderBenchmarks(): void {
  ensureBenchmarkSelection();
  setHTML(elements.benchmarkModelSelect, html`${benchmarkModels().map(model => html`
    <option value="${modelKey(model)}" ${modelKey(model) === state.benchmark.modelKey ? "selected" : ""}>
      ${modelLabel(model)}
    </option>
  `)}`);
  elements.benchmarkTypeSelect.value = state.benchmark.type;
  elements.benchmarkAllSections.checked = selectedAllSections();
  setHTML(elements.benchmarkSections, html`${benchmarkSections.map(section => html`
    <label class="toggle-row">
      <input type="checkbox" value="${section}" data-operation-group="benchmark" data-benchmark-section="${section}" ${state.benchmark.sections.includes(section) ? "checked" : ""} ${state.benchmark.type === "general" || selectedAllSections() ? "disabled data-unavailable" : ""}>
      <span>${section}</span>
    </label>
  `)}`);
  setControlUnavailable(elements.runBenchmarkButton, state.benchmark.running || !selectedModel());
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
    setHTML(elements.benchmarkLatest, html`<div class="error-text">${state.benchmark.error}</div>`);
    return;
  }
  if (!latest) {
    setHTML(elements.benchmarkLatest, html`<div class="card empty-state">No benchmark data for this config yet.</div>`);
    return;
  }
  const sections = benchmarkSections
    .map(section => record?.sections?.[section])
    .filter((summary): summary is BenchmarkSummary => Boolean(summary));
  setHTML(elements.benchmarkLatest, html`${[
    summaryCard("Latest", latest),
    ...sections.map(summary => summaryCard(summary.section, summary))
  ]}`);
}

function renderBenchmarkHistory(): void {
  const history = currentBenchmarkRecord()?.history ?? [];
  if (history.length === 0) {
    setHTML(elements.benchmarkHistory, html`<div class="empty-state">No history yet</div>`);
    return;
  }
  setHTML(elements.benchmarkHistory, html`${history.slice().reverse().map(summary => html`
    <article class="benchmark-row">
      <div>
        <strong>${summary.section}</strong>
        <div class="muted">${formatDate(summary.finished_at)} · ${summary.duration_ms || 0} ms</div>
      </div>
      ${badge(summary.status, benchmarkStatusTone(summary.status))}
      <div class="badge-row">${optionChanges(summary)}</div>
    </article>
  `)}`);
}

function summaryCard(title: string, summary: BenchmarkSummary): SafeHTML {
  return html`
    <article class="card benchmark-card">
      <header class="benchmark-card-head">
        <h3>${title}</h3>
        ${badge(summary.status, benchmarkStatusTone(summary.status))}
      </header>
      <p class="muted">${summary.duration_ms || 0} ms · ${formatDate(summary.finished_at)}</p>
      ${summary.error ? html`<p class="error-text">${summary.error}</p>` : ""}
      <dl class="fact-grid">${(summary.metrics ?? []).map(metric => fact(metric.name, formatMetricValue(metric)))}</dl>
    </article>
  `;
}

function benchmarkStatusTone(status: string): Tone {
  switch (status) {
    case "success":
      return "success";
    case "failed":
      return "danger";
    case "partial":
      return "warning";
    default:
      return "neutral";
  }
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

function optionChanges(summary: BenchmarkSummary): SafeHTML {
  const changes = summary.option_changes ?? [];
  if (changes.length === 0) {
    return html`<span class="muted">no option changes</span>`;
  }
  return html`${changes.map(change => badge(`${change.key} ${change.kind}`, "warning"))}`;
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
