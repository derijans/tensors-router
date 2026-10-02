import {
  addOption,
  addPayload,
  clearConstructor,
  clearLane,
  editLaneFields,
  removeOption,
  renderConstructor,
  toggleInspectorList,
  updateConstructorBackendMode,
  updateLaneTarget,
  updateOptionInput
} from "../constructor";
import { closeFieldEditor, handleFieldEditorClick, handleFieldEditorInput } from "../constructor-field-editor";
import { confirmDestructive } from "../dialogs";
import { markConstructorClean } from "../dirty-state";
import { closestElement, elementTarget, queryElements } from "../dom";
import { elements } from "../elements";
import { state } from "../state";
import { runTask } from "../tasks";
import type { PaletteName } from "../types";

export function bindConstructor(): void {
  queryElements("[data-palette]", HTMLButtonElement).forEach(button => {
    button.addEventListener("click", () => activatePalette(button.dataset.palette));
  });
  elements.constructorFilterInput.addEventListener("input", renderConstructor);
  elements.clearConstructorButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDestructive("Clear constructor?", "All selected lanes and option changes will be discarded.", "Clear")) {
      clearConstructor();
      markConstructorClean();
    }
  }, "constructor-clear", "cook-selection", "Clearing constructor…"));
  elements.advancedBackendSelect.addEventListener("change", () => updateConstructorBackendMode(elements.advancedBackendSelect.value));
  bindPalette();
  bindLanes();
  bindInspector();
  bindFieldDialog();
}

function activatePalette(name: string | undefined): void {
  if (!isPaletteName(name)) {
    return;
  }
  state.activePalette = name;
  queryElements("[data-palette]", HTMLButtonElement).forEach(tab => tab.classList.toggle("active", tab.dataset.palette === name));
  renderConstructor();
}

function isPaletteName(value: string | undefined): value is PaletteName {
  return value === "configs" || value === "files" || value === "options";
}

function bindPalette(): void {
  elements.paletteList.addEventListener("dragstart", event => {
    if (!(event instanceof DragEvent)) {
      return;
    }
    const payloadID = closestElement(event.target, "[data-drag-payload]", HTMLElement)?.dataset.dragPayload;
    if (!payloadID || !event.dataTransfer) {
      return;
    }
    event.dataTransfer.setData("text/plain", payloadID);
    event.dataTransfer.effectAllowed = "copy";
  });
  elements.paletteList.addEventListener("click", event => {
    const target = elementTarget(event);
    const optionKey = target?.dataset.addOption;
    if (optionKey) {
      addOption(optionKey);
      return;
    }
    const payloadID = target?.dataset.selectPayload;
    if (payloadID) {
      addPayload(state.palettePayloads[payloadID]);
    }
  });
}

function bindLanes(): void {
  elements.constructorLanes.addEventListener("dragover", event => {
    const drop = closestElement(event.target, "[data-drop-lane]", HTMLElement);
    if (!drop) {
      return;
    }
    event.preventDefault();
    drop.classList.add("drag-over");
  });
  elements.constructorLanes.addEventListener("dragleave", event => {
    closestElement(event.target, "[data-drop-lane]", HTMLElement)?.classList.remove("drag-over");
  });
  elements.constructorLanes.addEventListener("drop", event => {
    if (!(event instanceof DragEvent)) {
      return;
    }
    const drop = closestElement(event.target, "[data-drop-lane]", HTMLElement);
    if (!drop || !event.dataTransfer) {
      return;
    }
    event.preventDefault();
    drop.classList.remove("drag-over");
    addPayload(state.palettePayloads[event.dataTransfer.getData("text/plain")], drop.dataset.dropLane);
  });
  elements.constructorLanes.addEventListener("click", event => {
    const target = elementTarget(event);
    const clearLaneName = target?.dataset.clearLane;
    if (clearLaneName) {
      runTask(async () => {
        if (await confirmDestructive("Clear lane?", `The ${clearLaneName} selection and its overrides will be discarded.`, "Clear lane")) {
          clearLane(clearLaneName);
        }
      }, `lane-clear-${clearLaneName}`, "cook-selection", "Clearing lane…");
      return;
    }
    const editLaneName = target?.dataset.editLaneFields;
    if (editLaneName) {
      editLaneFields(editLaneName);
    }
  });
  elements.constructorLanes.addEventListener("change", event => updateLaneTarget(event.target));
}

function bindInspector(): void {
  elements.selectedOptionsList.addEventListener("input", event => updateOptionInput(event.target));
  elements.selectedOptionsList.addEventListener("change", event => updateOptionInput(event.target));
  elements.selectedOptionsList.addEventListener("click", event => {
    const target = elementTarget(event);
    const removeKey = target?.dataset.removeOption;
    if (removeKey) {
      removeOption(removeKey);
      return;
    }
    const toggle = target?.dataset.toggleList;
    if (toggle) {
      toggleInspectorList(toggle);
    }
  });
  elements.usedModelsList.addEventListener("click", event => {
    const toggle = elementTarget(event)?.dataset.toggleList;
    if (toggle) {
      toggleInspectorList(toggle);
    }
  });
}

function bindFieldDialog(): void {
  elements.constructorFieldDialog.addEventListener("cancel", event => {
    event.preventDefault();
    closeFieldEditor();
  });
  elements.constructorFieldDialog.addEventListener("click", event => {
    handleFieldEditorClick(event.target, renderConstructor);
  });
  elements.constructorFieldDialog.addEventListener("change", event => {
    handleFieldEditorInput(event.target);
  });
}
