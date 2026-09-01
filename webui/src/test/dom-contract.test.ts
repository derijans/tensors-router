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

  it("carries a Separate column and its dialog scaffold", () => {
    expect(page).toMatch(/<th>Routing<\/th>\s*<th>Separate<\/th>/);
    expect(tagNameForID("separateRuntimeDialog")).toBe("dialog");
    expect(tagNameForID("separateRuntimeDialogBody")).toBe("div");
    expect(tagNameForID("separateRuntimeDialogStatus")).toBe("p");
  });
});
