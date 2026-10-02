import { deleteRecipe } from "../api";
import { refreshInventory } from "../app-data";
import { invalidateAcceptedConversions } from "../conversions";
import { applyAdvancedCook, previewAdvancedCook } from "../cook-actions";
import { confirmDestructive } from "../dialogs";
import { confirmDiscardDirtyWork } from "../dirty-state";
import { elementTarget, queryElements } from "../dom";
import { elements } from "../elements";
import { renderRecipes } from "../render-dashboard";
import {
  addSelectedSimpleField,
  applySimpleCook,
  copySimpleConfig,
  deleteSimpleConfig,
  exportSimpleConfig,
  importSimpleConfig,
  newSimpleConfig,
  previewSimpleCook,
  removeSimpleField,
  renderSimpleCook,
  selectSimpleConfig,
  selectSimpleNode,
  showSimpleFieldValues,
  updateSimpleField,
  updateSimpleFieldFilter,
  updateSimpleSectionOpen
} from "../simple-cook";
import { state } from "../state";
import { runTask } from "../tasks";
import type { CookMode } from "../types";

export function bindCookModes(): void {
  queryElements("[data-cook-mode]", HTMLButtonElement).forEach(button => {
    button.addEventListener("click", () => activateCookMode(button.dataset.cookMode));
  });
  elements.cookIdInput.addEventListener("input", invalidateAcceptedConversions);
  elements.advancedCookIdInput.addEventListener("input", invalidateAcceptedConversions);
  elements.advancedPreviewButton.addEventListener("click", () => runTask(previewAdvancedCook, "constructor-preview", "cook", "Preparing preview…"));
  elements.advancedApplyButton.addEventListener("click", () => runTask(() => applyAdvancedCook(refreshInventory), "constructor-apply", "cook", "Applying cook plan…"));
}

export function bindSimpleCook(): void {
  elements.previewButton.addEventListener("click", () => runTask(previewSimpleCook, "quick-preview", "cook", "Preparing preview…"));
  elements.simpleExportButton.addEventListener("click", () => runTask(exportSimpleConfig, "quick-export", "cook", "Exporting KCPPS…"));
  elements.simpleImportButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDiscardDirtyWork("Importing a configuration")) {
      elements.simpleImportInput.click();
    }
  }, "quick-import-pick", "cook-selection", "Choosing file…"));
  elements.simpleImportInput.addEventListener("change", () => runTask(async () => {
    const file = elements.simpleImportInput.files?.[0];
    elements.simpleImportInput.value = "";
    if (file) {
      await importSimpleConfig(file);
    }
  }, "quick-import", "cook", "Importing KCPPS…"));
  elements.cookForm.addEventListener("submit", event => {
    event.preventDefault();
    runTask(() => applySimpleCook(refreshInventory), "quick-apply", "cook", "Applying config…");
  });
  elements.simpleNodeSelect.addEventListener("change", () => runTask(async () => {
    if (await confirmDiscardDirtyWork("Changing nodes")) {
      selectSimpleNode(elements.simpleNodeSelect.value);
    } else {
      renderSimpleCook();
    }
  }, "quick-node-change", "cook-selection", "Changing node…"));
  elements.simpleConfigSelect.addEventListener("change", () => runTask(async () => {
    if (await confirmDiscardDirtyWork("Changing configurations")) {
      selectSimpleConfig(elements.simpleConfigSelect.value);
    } else {
      renderSimpleCook();
    }
  }, "quick-config-change", "cook-selection", "Changing config…"));
  elements.simpleFieldFilter.addEventListener("input", () => updateSimpleFieldFilter(elements.simpleFieldFilter.value));
  elements.simpleAddFieldButton.addEventListener("click", addSelectedSimpleField);
  elements.simpleNewButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDiscardDirtyWork("Creating a new configuration")) {
      newSimpleConfig();
    }
  }, "quick-new", "cook-selection", "Opening new config…"));
  elements.simpleCopyButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDiscardDirtyWork("Copying this configuration")) {
      copySimpleConfig();
    }
  }, "quick-copy", "cook-selection", "Copying config…"));
  elements.simpleDeleteButton.addEventListener("click", () => runTask(() => deleteSimpleConfig(refreshInventory), "quick-delete", "cook", "Deleting config…"));
  elements.simpleConfigEditor.addEventListener("change", event => updateSimpleField(event.target));
  elements.simpleConfigEditor.addEventListener("toggle", event => updateSimpleSectionOpen(event.target), true);
  elements.simpleConfigEditor.addEventListener("click", handleSimpleEditorClick);
  elements.simpleFieldSidebar.addEventListener("click", event => {
    if (elementTarget(event)?.closest("[data-close-field-sidebar]")) {
      state.simpleCook.sidebar = null;
      renderSimpleCook();
    }
  });
}

export function bindRecipes(): void {
  elements.recipesList.addEventListener("click", event => {
    const id = elementTarget(event)?.closest<HTMLElement>("[data-delete-recipe]")?.dataset.deleteRecipe;
    if (id) {
      runTask(() => deleteConfirmedRecipe(id), "recipe-delete", "cook", "Deleting recipe…");
    }
  });
}

export async function editConfigInCook(nodeID: string, configID: string): Promise<void> {
  if (!await confirmDiscardDirtyWork("Opening another configuration")) {
    return;
  }
  activateCookMode("quick");
  selectSimpleNode(nodeID);
  selectSimpleConfig(configID);
}

function activateCookMode(name: string | undefined): void {
  if (!isCookMode(name)) {
    return;
  }
  state.activeCookMode = name;
  queryElements("[data-cook-mode]", HTMLButtonElement).forEach(tab => tab.classList.toggle("active", tab.dataset.cookMode === name));
  queryElements("[data-cook-panel]", HTMLElement).forEach(panel => panel.classList.toggle("active", panel.dataset.cookPanel === name));
}

function isCookMode(value: string | undefined): value is CookMode {
  return value === "quick" || value === "constructor";
}

function handleSimpleEditorClick(event: Event): void {
  const target = elementTarget(event);
  const fieldKey = target?.dataset.fieldValues;
  if (fieldKey) {
    showSimpleFieldValues(fieldKey, "field");
    return;
  }
  const modelFieldKey = target?.dataset.fieldModelValues;
  if (modelFieldKey) {
    showSimpleFieldValues(modelFieldKey, "model");
    return;
  }
  const removeKey = target?.dataset.removeSimpleField;
  if (removeKey) {
    removeSimpleField(removeKey);
  }
}

async function deleteConfirmedRecipe(id: string): Promise<void> {
  if (!await confirmDestructive("Delete recipe?", `Delete ${id}? This removes the public split route.`, "Delete")) {
    return;
  }
  await deleteRecipe(id);
  await refreshInventory();
  renderRecipes();
}
