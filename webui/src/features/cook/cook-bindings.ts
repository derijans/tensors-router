import { deleteRecipe } from "../../api/cook-api";
import { refreshInventory } from "../shell/refresh";
import { invalidateAcceptedConversions } from "./conversions";
import { applyAdvancedCook, previewAdvancedCook } from "./constructor/cook-actions";
import { confirmDestructive } from "../shell/dialogs";
import { confirmDiscardDirtyWork } from "../shell/dirty-state";
import { elementTarget, queryElements } from "../../lib/dom";
import { renderRecipes } from "../shell/render-dashboard";
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
  selectSimpleConfig,
  selectSimpleNode,
  showSimpleFieldValues,
  updateSimpleField,
  updateSimpleFieldFilter,
  updateSimpleSectionOpen
} from "./simple-cook/simple-cook";
import { renderSimpleCook } from "./simple-cook/simple-cook-render";
import { state } from "../../state/app-state";
import { runTask } from "../shell/tasks";
import type { CookMode } from "../../types";
import { cookElements } from "./cook-elements";

export function bindCookModes(): void {
  queryElements("[data-cook-mode]", HTMLButtonElement).forEach(button => {
    button.addEventListener("click", () => activateCookMode(button.dataset.cookMode));
  });
  cookElements.cookIdInput.addEventListener("input", invalidateAcceptedConversions);
  cookElements.advancedCookIdInput.addEventListener("input", invalidateAcceptedConversions);
  cookElements.advancedPreviewButton.addEventListener("click", () => runTask(previewAdvancedCook, "constructor-preview", "cook", "Preparing preview…"));
  cookElements.advancedApplyButton.addEventListener("click", () => runTask(() => applyAdvancedCook(refreshInventory), "constructor-apply", "cook", "Applying cook plan…"));
}

export function bindSimpleCook(): void {
  cookElements.previewButton.addEventListener("click", () => runTask(previewSimpleCook, "quick-preview", "cook", "Preparing preview…"));
  cookElements.simpleExportButton.addEventListener("click", () => runTask(exportSimpleConfig, "quick-export", "cook", "Exporting KCPPS…"));
  cookElements.simpleImportButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDiscardDirtyWork("Importing a configuration")) {
      cookElements.simpleImportInput.click();
    }
  }, "quick-import-pick", "cook-selection", "Choosing file…"));
  cookElements.simpleImportInput.addEventListener("change", () => runTask(async () => {
    const file = cookElements.simpleImportInput.files?.[0];
    cookElements.simpleImportInput.value = "";
    if (file) {
      await importSimpleConfig(file);
    }
  }, "quick-import", "cook", "Importing KCPPS…"));
  cookElements.cookForm.addEventListener("submit", event => {
    event.preventDefault();
    runTask(() => applySimpleCook(refreshInventory), "quick-apply", "cook", "Applying config…");
  });
  cookElements.simpleNodeSelect.addEventListener("change", () => runTask(async () => {
    if (await confirmDiscardDirtyWork("Changing nodes")) {
      selectSimpleNode(cookElements.simpleNodeSelect.value);
    } else {
      renderSimpleCook();
    }
  }, "quick-node-change", "cook-selection", "Changing node…"));
  cookElements.simpleConfigSelect.addEventListener("change", () => runTask(async () => {
    if (await confirmDiscardDirtyWork("Changing configurations")) {
      selectSimpleConfig(cookElements.simpleConfigSelect.value);
    } else {
      renderSimpleCook();
    }
  }, "quick-config-change", "cook-selection", "Changing config…"));
  cookElements.simpleFieldFilter.addEventListener("input", () => updateSimpleFieldFilter(cookElements.simpleFieldFilter.value));
  cookElements.simpleAddFieldButton.addEventListener("click", addSelectedSimpleField);
  cookElements.simpleNewButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDiscardDirtyWork("Creating a new configuration")) {
      newSimpleConfig();
    }
  }, "quick-new", "cook-selection", "Opening new config…"));
  cookElements.simpleCopyButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDiscardDirtyWork("Copying this configuration")) {
      copySimpleConfig();
    }
  }, "quick-copy", "cook-selection", "Copying config…"));
  cookElements.simpleDeleteButton.addEventListener("click", () => runTask(() => deleteSimpleConfig(refreshInventory), "quick-delete", "cook", "Deleting config…"));
  cookElements.simpleConfigEditor.addEventListener("change", event => updateSimpleField(event.target));
  cookElements.simpleConfigEditor.addEventListener("toggle", event => updateSimpleSectionOpen(event.target), true);
  cookElements.simpleConfigEditor.addEventListener("click", handleSimpleEditorClick);
  cookElements.simpleFieldSidebar.addEventListener("click", event => {
    if (elementTarget(event)?.closest("[data-close-field-sidebar]")) {
      state.simpleCook.sidebar = null;
      renderSimpleCook();
    }
  });
}

export function bindRecipes(): void {
  cookElements.recipesList.addEventListener("click", event => {
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
