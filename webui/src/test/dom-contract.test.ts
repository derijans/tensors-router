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

describe("security tab DOM contract", () => {
  it("renders the router picker as a select inside a section that starts hidden", () => {
    expect(tagNameForID("securityNodeSelect")).toBe("select");
    expect(page).toMatch(/<div id="securityNodePicker"[^>]*\bhidden\b/);
  });

  it.each(["securityTokenRows", "securityCertificateRows", "securityPendingKeyRows"])("renders #%s as a table body", id => {
    expect(tagNameForID(id)).toBe("tbody");
  });

  it("starts the master-only cluster setup button and card hidden, with an ordered step list", () => {
    expect(page).toMatch(/<button id="securityWizardButton"[^>]*\bhidden\b/);
    expect(page).toMatch(/<article id="securityWizard"[^>]*\bhidden\b/);
    expect(tagNameForID("securityWizardSteps")).toBe("ol");
  });

  it.each(["securityPushClusterTokenButton", "securityIssueFromMasterButton", "securityTrustMasterButton"])("starts the slave-only #%s hidden", id => {
    expect(page).toMatch(new RegExp(`<button id="${id}"[^>]*\\bhidden\\b`));
  });
});
