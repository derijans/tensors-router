import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const page = readFileSync(new URL("../../index.html", import.meta.url), "utf8");

function tagNameForID(id: string): string | undefined {
  const element = page.match(new RegExp(`<([a-z][a-z0-9-]*)\\b[^>]*\\bid=["']${id}["'][^>]*>`, "i"));
  return element?.[1]?.toLowerCase();
}

describe("models dashboard DOM contract", () => {
  it.each(["modelsNodeFilter", "filesNodeFilter", "modelEnabledFilter", "modelBackendFilter", "modelCapabilityFilter", "fileRoleFilter", "fileExtensionFilter", "fileHashFilter"])("renders #%s as a select", id => {
    expect(tagNameForID(id)).toBe("select");
  });

  it("renders independent full-width Models and Files subpanels without inline styles", () => {
    expect(page).toContain('data-model-inventory-subtab="models"');
    expect(page).toContain('data-model-inventory-subtab="files"');
    expect(page).toContain('data-model-inventory-panel="models"');
    expect(page).toContain('data-model-inventory-panel="files"');
    expect(page).not.toMatch(/\sstyle=/i);
  });

  it("carries the separate runtime dialog scaffold and a header the column renderer fills", () => {
    expect(tagNameForID("modelsTableHead")).toBe("thead");
    expect(tagNameForID("separateRuntimeDialog")).toBe("dialog");
    expect(tagNameForID("separateRuntimeDialogBody")).toBe("div");
    expect(tagNameForID("separateRuntimeDialogStatus")).toBe("p");
  });
});

describe("lending tab DOM contract", () => {
  it.each(["lendingWindowSelect", "lendingLaneSelect", "lendingOutcomeSelect"])("renders #%s as a select", id => {
    expect(tagNameForID(id)).toBe("select");
  });

  it.each(["lendingSettingsRows", "lendingDecisionRows"])("renders #%s as a table body", id => {
    expect(tagNameForID(id)).toBe("tbody");
  });

  it("offers the Lending tab next to its panel", () => {
    expect(page).toContain('data-tab="lending"');
    expect(page).toContain('data-panel="lending"');
  });
});
