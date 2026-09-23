import { SafeHTML, emptyHTML, html } from "./safe-html";
import type { ConfigFileResponse, CookResponse, ErrorResponse } from "./types";

export function cookResultHTML(value: CookResponse | ConfigFileResponse | ErrorResponse): SafeHTML {
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
      html`${configs.map(config => html`<li>${config.node_id} / ${config.filename} / ${config.kinds.join(", ")}${config.would_overwrite ? " / overwrite" : ""}</li>`)}`,
      html`${validation.map(issue => html`<li>${issue.severity} / ${issue.field || issue.code} / ${issue.message}</li>`)}`,
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
      emptyHTML,
      emptyHTML,
      value
    );
  }
  const error = typeof value.error === "string" ? value.error : value.error?.message || "Operation failed";
  return resultShell("Operation failed", [["Error", error]], emptyHTML, html`${(value.validation ?? []).map(issue => html`<li>${issue.message}</li>`)}`, value);
}

function isCookResponse(value: CookResponse | ConfigFileResponse | ErrorResponse): value is CookResponse {
  return "plan" in value;
}

function isConfigFileResponse(value: CookResponse | ConfigFileResponse | ErrorResponse): value is ConfigFileResponse {
  return "id" in value && "filename" in value;
}

function resultShell(title: string, facts: Array<[string, string]>, items: SafeHTML, validation: SafeHTML, raw: unknown): SafeHTML {
  return html`
    <section class="cook-result-grid">
      <h3>${title}</h3>
      <div class="status-grid">
        ${facts.map(([label, value]) => html`<div class="status-item"><div class="status-label">${label}</div><div class="status-value">${value}</div></div>`)}
      </div>
      ${items.isEmpty() ? "" : html`<ul>${items}</ul>`}
      ${validation.isEmpty() ? "" : html`<div><strong>Validation</strong><ul>${validation}</ul></div>`}
      <details><summary>Raw diagnostic</summary><pre>${JSON.stringify(raw, null, 2)}</pre></details>
    </section>
  `;
}
