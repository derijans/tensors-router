import { escapeHTML } from "./utils";
import type { ConfigFileResponse, CookResponse, ErrorResponse } from "./types";

export function cookResultHTML(value: CookResponse | ConfigFileResponse | ErrorResponse): string {
  if (isCookResponse(value)) {
    const configs = value.plan.configs ?? [];
    const validation = value.validation ?? [];
    return resultShell(
      "Cook plan",
      [
        ["Public model", value.plan.public_id],
        ["Public image", value.plan.public_image_id || "none"],
        ["Configs", String(configs.length)],
        ["Master recipe", value.plan.requires_master_recipe ? "required" : "not required"]
      ],
      `${configs.map(config => `<li>${escapeHTML(config.node_id)} / ${escapeHTML(config.filename)} / ${escapeHTML(config.kinds.join(", "))}${config.would_overwrite ? " / overwrite" : ""}</li>`).join("")}`,
      validation.map(issue => `<li>${escapeHTML(issue.severity)} / ${escapeHTML(issue.field || issue.code)} / ${escapeHTML(issue.message)}</li>`).join(""),
      value
    );
  }
  if (isConfigFileResponse(value)) {
    return resultShell(
      value.deleted ? "Config deleted" : "Config ready",
      [
        ["Node", value.node_id],
        ["Config", value.id],
        ["File", value.filename],
        ["Overwrite", value.would_overwrite ? "yes" : "no"]
      ],
      "",
      "",
      value
    );
  }
  const error = typeof value.error === "string" ? value.error : value.error?.message || "Operation failed";
  return resultShell("Operation failed", [["Error", error]], "", (value.validation ?? []).map(issue => `<li>${escapeHTML(issue.message)}</li>`).join(""), value);
}

function isCookResponse(value: CookResponse | ConfigFileResponse | ErrorResponse): value is CookResponse {
  return "plan" in value;
}

function isConfigFileResponse(value: CookResponse | ConfigFileResponse | ErrorResponse): value is ConfigFileResponse {
  return "id" in value && "filename" in value;
}

function resultShell(title: string, facts: Array<[string, string]>, items: string, validation: string, raw: unknown): string {
  return `
    <section class="cook-result-grid">
      <h3>${escapeHTML(title)}</h3>
      <div class="status-grid">
        ${facts.map(([label, value]) => `<div class="status-item"><div class="status-label">${escapeHTML(label)}</div><div class="status-value">${escapeHTML(value)}</div></div>`).join("")}
      </div>
      ${items ? `<ul>${items}</ul>` : ""}
      ${validation ? `<div><strong>Validation</strong><ul>${validation}</ul></div>` : ""}
      <details><summary>Raw diagnostic</summary><pre>${escapeHTML(JSON.stringify(raw, null, 2))}</pre></details>
    </section>
  `;
}
