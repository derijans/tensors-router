export function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>"']/g, character => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    "\"": "&quot;",
    "'": "&#39;"
  }[character]));
}

export function escapeAttribute(value) {
  return escapeHTML(value).replace(/`/g, "&#96;");
}

export function statusItem(label, value) {
  return `
    <div class="status-item">
      <div class="status-label">${escapeHTML(label)}</div>
      <div class="status-value">${escapeHTML(String(value))}</div>
    </div>
  `;
}

export function chip(label, color) {
  const value = String(label || "").trim();
  if (!value) {
    return "";
  }
  return `<span class="chip ${escapeAttribute(color || "")}">${escapeHTML(value)}</span>`;
}

export function renderIssue(item) {
  return `
    <div class="issue ${item.severity === "error" ? "error" : ""}">
      <strong>${escapeHTML(item.severity)} / ${escapeHTML(item.code)}</strong>
      <span>${escapeHTML(item.message)}</span>
    </div>
  `;
}

export function issue(severity, code, message, field) {
  return {severity, code, message, field};
}

export function kindColor(kind) {
  switch (kind) {
    case "image":
      return "magenta";
    case "embeddings":
      return "lime";
    default:
      return "cyan";
  }
}

export function capabilities(model) {
  const values = [];
  if (model.has_llm) values.push("llm");
  if (model.has_image) values.push("image");
  if (model.has_embeddings) values.push("embeddings");
  if (model.has_multimodal) values.push("multimodal");
  return values.join(", ") || "none";
}

export function optionSummary(options) {
  const count = Object.keys(options || {}).length;
  return count ? `${count} filled` : "none";
}

export function fileRoles(file) {
  return file.roles || [file.role || "unknown"];
}

export function hasKind(components, kind) {
  return components.some(component => component.kind === kind);
}

export function gpuOptionKey(key) {
  const lowered = String(key).toLowerCase();
  return ["gpulayers", "tensor_split", "maingpu", "usecuda", "usecublas", "embeddingsgpu", "sdclipgpu", "sdflashattention"].includes(key) ||
    lowered.includes("gpu") ||
    lowered.includes("cuda");
}

export function highGPUOption(key, value) {
  if (["gpulayers", "maingpu"].includes(key)) {
    return numberOption(value) > 0;
  }
  if (["tensor_split", "usecuda", "usecublas", "embeddingsgpu", "sdclipgpu", "sdflashattention"].includes(key)) {
    return truthy(value);
  }
  return gpuOptionKey(key) && truthy(value);
}

export function truthy(value) {
  if (typeof value === "boolean") return value;
  if (typeof value === "number") return value !== 0;
  if (typeof value === "string") return value.trim() !== "";
  return value !== null && value !== undefined;
}

export function numberOption(value) {
  if (typeof value === "number") return value;
  if (typeof value === "string") {
    const parsed = Number.parseInt(value, 10);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

export function optionInputValue(value) {
  if (typeof value === "string") {
    return value;
  }
  return JSON.stringify(value);
}

export function optionValueLabel(value) {
  if (typeof value === "string") {
    return value;
  }
  return JSON.stringify(value);
}

export function parseOptionInput(definition, value) {
  const trimmed = value.trim();
  switch (definition?.value_type) {
    case "bool":
      return trimmed === "true" || trimmed === "1" || trimmed === "yes";
    case "number": {
      const number = Number(trimmed);
      return Number.isFinite(number) ? number : 0;
    }
    case "json":
      if (!trimmed) {
        return {};
      }
      return JSON.parse(trimmed);
    default:
      return value;
  }
}

export function formatBytes(value) {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`;
  return `${(value / 1024 / 1024 / 1024).toFixed(1)} GB`;
}
