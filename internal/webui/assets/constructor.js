import { elements } from "./elements.js";
import { emptyLanes, state } from "./state.js";
import {
  configPaletteEntries,
  filePaletteEntries,
  groupComponentsByNode,
  optionDefinition,
  optionPaletteEntries,
  nodeByID,
  selectedOptionsForInspector,
  selectedOptionsForNode,
  selectedThreadsForNode,
  usedPaths
} from "./data.js";
import {
  chip,
  escapeAttribute,
  escapeHTML,
  gpuOptionKey,
  hasKind,
  issue,
  kindColor,
  optionInputValue,
  optionValueLabel,
  parseOptionInput,
  renderIssue,
  truthy
} from "./utils.js";

export function renderConstructor() {
  renderPalette();
  renderLanes();
  renderInspector();
}

export function advancedCookRequest() {
  const components = Object.values(state.constructor.lanes)
    .filter(Boolean)
    .map(selected => selected.component);
  return {
    id: elements.advancedCookIdInput.value.trim(),
    overwrite: elements.overwriteInput.checked,
    components,
    options: {...state.constructor.options}
  };
}

export function localValidation() {
  const issues = [];
  const request = advancedCookRequest();
  if (!request.id) {
    issues.push(issue("warning", "id_missing", "Config id is empty.", "id"));
  }
  if (request.components.length === 0) {
    issues.push(issue("warning", "empty_constructor", "No lanes selected.", ""));
  }
  for (const [nodeID, components] of groupComponentsByNode(request.components)) {
    const node = nodeByID(nodeID);
    const backend = node?.backend_mode || "kobold";
    if (backend === "kobold" && hasKind(components, "image") && hasKind(components, "embeddings")) {
      issues.push(issue("error", "kobold_image_embeddings_mix", "Kobold cannot cook image and embeddings into the same config.", nodeID));
    }
    const threads = selectedThreadsForNode(nodeID, components, request.options);
    const maxThreads = node?.hardware?.max_threads || 0;
    if (threads > 0 && maxThreads > 0 && threads > maxThreads) {
      issues.push(issue("error", "thread_budget_exceeded", `${threads} selected threads exceed ${maxThreads} logical CPUs.`, nodeID));
    }
    const selected = selectedOptionsForNode(nodeID, components, request.options);
    if (node?.hardware?.gpu_backend === "rocm") {
      for (const [key, value] of Object.entries(selected)) {
        if (optionDefinition(key)?.cuda_only && truthy(value)) {
          issues.push(issue("error", "cuda_on_rocm", `${key} is CUDA-only on a ROCm node.`, key));
        }
      }
    }
    if (!node?.hardware?.gpu_backend || node.hardware.gpu_backend === "unknown") {
      for (const [key, value] of Object.entries(selected)) {
        if (gpuOptionKey(key) && truthy(value)) {
          issues.push(issue("warning", "gpu_backend_unknown", "GPU backend could not be inferred.", nodeID));
          break;
        }
      }
    }

    for (const [key, value] of Object.entries(selected)) {
      const definition = optionDefinition(key);
      if (!definition?.known) {
        issues.push(issue("warning", "unverified_option", `${key} is observed but not verified.`, key));
        continue;
      }
      if (definition.backends?.length > 0 && !definition.backends.includes(backend)) {
        issues.push(issue("warning", "unsupported_option", `${key} is not marked as supported by ${backend}.`, key));
      }
    }
  }
  return issues;
}

export function addPayload(payload, lane) {
  if (!payload) {
    return;
  }
  if (payload.type === "option") {
    addOption(payload.key);
    return;
  }
  if (payload.type !== "component") {
    return;
  }
  const targetLane = lane || payload.lane;
  if (targetLane !== payload.lane) {
    return;
  }
  state.constructor.lanes[targetLane] = payload;
  renderConstructor();
}

export function addOption(key) {
  const definition = optionDefinition(key);
  if (!definition) {
    return;
  }
  if (!Object.hasOwn(state.constructor.options, key)) {
    state.constructor.options[key] = defaultOptionValue(definition);
  }
  renderConstructor();
}

export function clearConstructor() {
  state.constructor.lanes = emptyLanes();
  state.constructor.options = {};
  renderConstructor();
}

export function clearLane(lane) {
  state.constructor.lanes[lane] = null;
  renderConstructor();
}

export function updateOptionInput(target) {
  const key = target?.dataset?.optionInput;
  if (!key) {
    return;
  }
  try {
    state.constructor.options[key] = parseOptionInput(optionDefinition(key), target.value);
    target.setCustomValidity("");
    renderValidation();
  } catch {
    target.setCustomValidity("Invalid JSON");
    target.reportValidity();
  }
}

export function removeOption(key) {
  delete state.constructor.options[key];
  renderConstructor();
}

export function toggleInspectorList(target) {
  if (target === "used") {
    state.constructor.showUsedAll = !state.constructor.showUsedAll;
  }
  if (target === "options") {
    state.constructor.showOptionsAll = !state.constructor.showOptionsAll;
  }
  renderInspector();
}

function renderPalette() {
  const query = elements.constructorFilterInput.value.trim().toLowerCase();
  const entries = paletteEntries().filter(entry => !query || JSON.stringify(entry).toLowerCase().includes(query));
  state.palettePayloads = {};
  elements.paletteList.innerHTML = entries.map(entry => {
    const payloadID = `payload-${Object.keys(state.palettePayloads).length}`;
    state.palettePayloads[payloadID] = entry.payload;
    const addButton = entry.payload.type === "option"
      ? `<button type="button" data-add-option="${escapeAttribute(entry.payload.key)}">Add</button>`
      : `<button type="button" data-select-payload="${escapeAttribute(payloadID)}">Use</button>`;
    return `
      <article class="palette-item" draggable="true" data-drag-payload="${escapeAttribute(payloadID)}">
        <div class="palette-title">
          <strong>${escapeHTML(entry.title)}</strong>
          ${chip(entry.badge, entry.color)}
        </div>
        <div class="muted">${escapeHTML(entry.subtitle)}</div>
        <div class="palette-meta">${entry.meta.map(item => chip(item, "")).join("")}</div>
        ${addButton}
      </article>
    `;
  }).join("") || `<div class="detail-empty">No items</div>`;
}

function renderLanes() {
  for (const lane of ["text", "image", "embeddings"]) {
    const drop = document.querySelector(`[data-drop-lane="${lane}"]`);
    const selected = state.constructor.lanes[lane];
    if (!selected) {
      drop.innerHTML = `<div class="lane-empty">Drop ${escapeHTML(lane)} here</div>`;
      continue;
    }
    drop.innerHTML = `
      <article class="selected-card">
        <strong>${escapeHTML(selected.label)}</strong>
        <div class="muted">${escapeHTML(selected.subtitle)}</div>
        <div class="palette-meta">${selected.meta.map(item => chip(item, "")).join("")}</div>
      </article>
    `;
  }
}

function renderInspector() {
  renderValidation();
  const used = usedModelRows();
  elements.usedModelsList.innerHTML = limitedRows(used, state.constructor.showUsedAll, "used").join("") || `<div class="detail-empty">No models selected</div>`;
  const options = selectedOptionRows();
  elements.selectedOptionsList.innerHTML = limitedRows(options, state.constructor.showOptionsAll, "options").join("") || `<div class="detail-empty">No options selected</div>`;
}

function renderValidation() {
  const validation = localValidation();
  elements.validationList.innerHTML = validation.length
    ? validation.map(renderIssue).join("")
    : `<div class="detail-empty">Clean</div>`;
}

function paletteEntries() {
  if (state.activePalette === "files") {
    return filePaletteEntries();
  }
  if (state.activePalette === "options") {
    return optionPaletteEntries();
  }
  return configPaletteEntries();
}

function usedModelRows() {
  const rows = [];
  for (const lane of ["text", "image", "embeddings"]) {
    const selected = state.constructor.lanes[lane];
    if (!selected) {
      continue;
    }
    rows.push(`
      <div class="used-row">
        ${chip(lane, kindColor(lane))}
        <span>${escapeHTML(selected.label)}</span>
      </div>
    `);
    for (const value of usedPaths(selected)) {
      rows.push(`<div class="muted">${escapeHTML(value)}</div>`);
    }
  }
  return rows;
}

function selectedOptionRows() {
  const rows = [];
  const merged = selectedOptionsForInspector();
  for (const [key, value] of Object.entries(merged).sort(([left], [right]) => left.localeCompare(right))) {
    if (Object.hasOwn(state.constructor.options, key)) {
      rows.push(optionEditorRow(key, state.constructor.options[key]));
    } else {
      rows.push(`
        <div class="option-row">
          ${chip(key, "")}
          <span class="muted">${escapeHTML(optionValueLabel(value))}</span>
        </div>
      `);
    }
  }
  return rows;
}

function optionEditorRow(key, value) {
  return `
    <div class="option-editor">
      <span>${escapeHTML(key)}</span>
      <input data-option-input="${escapeAttribute(key)}" value="${escapeAttribute(optionInputValue(value))}">
      <button type="button" data-remove-option="${escapeAttribute(key)}">Remove</button>
    </div>
  `;
}

function limitedRows(rows, showAll, target) {
  const limit = 9;
  if (rows.length <= limit || showAll) {
    if (rows.length > limit) {
      return [...rows, `<button class="link-button" type="button" data-toggle-list="${target}">Show less</button>`];
    }
    return rows;
  }
  return [
    ...rows.slice(0, limit),
    `<button class="link-button" type="button" data-toggle-list="${target}">Show all ${rows.length}</button>`
  ];
}

function defaultOptionValue(definition) {
  switch (definition.value_type) {
    case "bool":
      return false;
    case "number":
      return 0;
    case "json":
      return {};
    default:
      return "";
  }
}
