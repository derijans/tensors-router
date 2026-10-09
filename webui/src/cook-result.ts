import { SafeHTML, emptyHTML, html } from "./safe-html";
import { fact } from "./markup-primitives";
import type { ConfigFileResponse, CookResponse, ErrorResponse } from "./types";

type CookResultValue = CookResponse | ConfigFileResponse | ErrorResponse;

export function cookResultHTML(value: CookResultValue): SafeHTML {
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
      html`${configs.map(plannedConfigItem)}`,
      html`${validation.map(validationIssueItem)}`,
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
  return resultShell("Operation failed", [["Error", error]], emptyHTML, html`${(value.validation ?? []).map(issueMessageItem)}`, value);
}

type PlannedConfig = NonNullable<CookResponse["plan"]["configs"]>[number];
type ValidationIssueItem = NonNullable<CookResponse["validation"]>[number];

function plannedConfigItem(config: PlannedConfig): SafeHTML {
  return html`<li>${config.node_id} / ${config.filename} / ${config.kinds.join(", ")}${config.would_overwrite ? " / overwrite" : ""}</li>`;
}

function validationIssueItem(issue: ValidationIssueItem): SafeHTML {
  return html`<li>${issue.severity} / ${issue.field || issue.code} / ${issue.message}</li>`;
}

function issueMessageItem(issue: { message: string }): SafeHTML {
  return html`<li>${issue.message}</li>`;
}


function isCookResponse(value: CookResultValue): value is CookResponse {
  return "plan" in value;
}

function isConfigFileResponse(value: CookResultValue): value is ConfigFileResponse {
  return "id" in value && "filename" in value;
}

function resultShell(title: string, facts: Array<[string, string]>, items: SafeHTML, validation: SafeHTML, raw: unknown): SafeHTML {
  return html`
    <section class="cook-result-grid">
      <h3>${title}</h3>
      <dl class="fact-grid">
        ${facts.map(([label, value]) => fact(label, value))}
      </dl>
      ${items.isEmpty() ? "" : html`<ul>${items}</ul>`}
      ${validation.isEmpty() ? "" : html`<div><strong>Validation</strong><ul>${validation}</ul></div>`}
      <details><summary>Raw diagnostic</summary><pre>${JSON.stringify(raw, null, 2)}</pre></details>
    </section>
  `;
}
