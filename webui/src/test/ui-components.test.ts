import { describe, expect, it } from "vitest";
import { SafeHTML } from "../lib/safe-html";
import { attributes } from "../ui/attributes";
import { badgeButton } from "../ui/badge";
import { closeButton, primaryButton } from "../ui/buttons";
import { emptyTableRow } from "../ui/empty-state";
import { presentFactGrid } from "../ui/fact-grid";
import { checkbox } from "../ui/form";
import { progressBar } from "../ui/progress";
import { optionList } from "../ui/select";
import { nodeErrorText } from "../ui/text";

const hostile = `"><script>alert(1)</script>`;

function rendered(markup: SafeHTML): string {
  return SafeHTML.render(markup);
}

describe("attribute rendering", () => {
  it("escapes attribute values so they cannot close the quote", () => {
    expect(rendered(attributes({"data-node-id": hostile}))).toBe(' data-node-id="&quot;&gt;&lt;script&gt;alert(1)&lt;/script&gt;"');
  });

  it("renders true as a bare flag and omits false or missing values", () => {
    expect(rendered(attributes({disabled: true, checked: false, "data-profile": undefined}))).toBe(" disabled");
  });

  it("rejects attribute names that could inject markup or handlers through the name", () => {
    expect(() => attributes({"onclick=alert(1) x": "1"})).toThrow("Invalid attribute name");
    expect(() => attributes({"\"><b": "1"})).toThrow("Invalid attribute name");
  });
});

describe("button components", () => {
  it("escapes the accessible name and keeps the close glyph", () => {
    expect(rendered(closeButton(`Remove ${hostile}`, {"data-remove-simple-field": "key"}))).toBe(
      '<button type="button" class="icon-button" aria-label="Remove &quot;&gt;&lt;script&gt;alert(1)&lt;/script&gt;" data-remove-simple-field="key">×</button>'
    );
  });

  it("marks a primary button disabled when asked", () => {
    expect(rendered(primaryButton("Save links", {"data-routing-action": "save", disabled: true}))).toBe(
      '<button type="button" class="primary" data-routing-action="save" disabled>Save links</button>'
    );
  });

  it("builds a badge button from tone and extra class", () => {
    expect(rendered(badgeButton("backend needs init", "warning", {disabled: true}, "node-backend-init-action"))).toBe(
      '<button type="button" class="badge tone-warning node-backend-init-action" disabled>backend needs init</button>'
    );
  });
});

describe("form and table components", () => {
  it("reflects the checked state on a checkbox", () => {
    expect(rendered(checkbox({"data-model-column": "fit"}, true))).toBe('<input type="checkbox" data-model-column="fit" checked>');
    expect(rendered(checkbox({"data-model-column": "fit"}, false))).toBe('<input type="checkbox" data-model-column="fit">');
  });

  it("spans an empty table row across every column", () => {
    expect(rendered(emptyTableRow("No files match the current filters.", 6))).toBe(
      '<tr><td class="empty-state" colspan="6">No files match the current filters.</td></tr>'
    );
  });

  it("selects only the matching option", () => {
    expect(rendered(optionList([{value: "24h", label: "Last 24 hours"}, {value: "7d", label: "Last 7 days"}], "7d"))).toBe(
      '<option value="24h">Last 24 hours</option><option value="7d" selected>Last 7 days</option>'
    );
  });

  it("leaves out facts without a value", () => {
    expect(rendered(presentFactGrid([["Node", "node-a"], ["Router version", ""]]))).toBe(
      '<dl class="fact-grid"><div class="fact"><dt>Node</dt><dd>node-a</dd></div></dl>'
    );
  });
});

describe("progress bar", () => {
  it("clamps completed work into the bar range", () => {
    expect(rendered(progressBar(4096, 2048))).toBe('<progress value="2048" max="2048"></progress>');
    expect(rendered(progressBar(-5, 2048))).toBe('<progress value="0" max="2048"></progress>');
  });

  it("keeps a usable maximum when the total is still unknown", () => {
    expect(rendered(progressBar(0, 0))).toBe('<progress value="0" max="1"></progress>');
  });
});

describe("node error text", () => {
  it("falls back from node id to node url to a generic label", () => {
    expect(rendered(nodeErrorText({node_id: "node-a", error: "down"}))).toBe('<p class="error-text">node-a: down</p>');
    expect(rendered(nodeErrorText({node_url: "https://b", error: "down"}, "div"))).toBe('<div class="error-text">https://b: down</div>');
    expect(rendered(nodeErrorText({error: "down"}))).toBe('<p class="error-text">node: down</p>');
  });
});
