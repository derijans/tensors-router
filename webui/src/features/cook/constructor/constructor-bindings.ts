import {
  addOption,
  addPayload,
  clearConstructor,
  clearLane,
  editLaneFields,
  removeOption,
  toggleInspectorList,
  updateConstructorBackendMode,
  updateLaneTarget,
  updateOptionInput
} from "./constructor";
import { renderConstructor } from "./constructor-render";
import { closeFieldEditor, handleFieldEditorClick, handleFieldEditorInput } from "./constructor-field-editor";
import { confirmDestructive } from "../../shell/dialogs";
import { markConstructorClean } from "../../shell/dirty-state";
import { closestElement, elementTarget, queryElements } from "../../../lib/dom";
import { state } from "../../../state/app-state";
import { runTask } from "../../shell/tasks";
import type { PaletteName } from "../../../types";
import { cookElements } from "../cook-elements";

export function bindConstructor(): void {
  queryElements("[data-palette]", HTMLButtonElement).forEach(button => {
    button.addEventListener("click", () => activatePalette(button.dataset.palette));
  });
  cookElements.constructorFilterInput.addEventListener("input", renderConstructor);
  cookElements.clearConstructorButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDestructive("Clear constructor?", "All selected lanes and option changes will be discarded.", "Clear")) {
      clearConstructor();
      markConstructorClean();
    }
  }, "constructor-clear", "cook-selection", "Clearing constructor…"));
  cookElements.advancedBackendSelect.addEventListener("change", () => updateConstructorBackendMode(cookElements.advancedBackendSelect.value));
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
  cookElements.paletteList.addEventListener("dragstart", event => {
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
  cookElements.paletteList.addEventListener("click", event => {
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
  cookElements.constructorLanes.addEventListener("dragover", event => {
    const drop = closestElement(event.target, "[data-drop-lane]", HTMLElement);
    if (!drop) {
      return;
    }
    event.preventDefault();
    drop.classList.add("drag-over");
  });
  cookElements.constructorLanes.addEventListener("dragleave", event => {
    closestElement(event.target, "[data-drop-lane]", HTMLElement)?.classList.remove("drag-over");
  });
  cookElements.constructorLanes.addEventListener("drop", event => {
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
  cookElements.constructorLanes.addEventListener("click", event => {
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
  cookElements.constructorLanes.addEventListener("change", event => updateLaneTarget(event.target));
}

function bindInspector(): void {
  cookElements.selectedOptionsList.addEventListener("input", event => updateOptionInput(event.target));
  cookElements.selectedOptionsList.addEventListener("change", event => updateOptionInput(event.target));
  cookElements.selectedOptionsList.addEventListener("click", event => {
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
  cookElements.usedModelsList.addEventListener("click", event => {
    const toggle = elementTarget(event)?.dataset.toggleList;
    if (toggle) {
      toggleInspectorList(toggle);
    }
  });
}

function bindFieldDialog(): void {
  cookElements.constructorFieldDialog.addEventListener("cancel", event => {
    event.preventDefault();
    closeFieldEditor();
  });
  cookElements.constructorFieldDialog.addEventListener("click", event => {
    handleFieldEditorClick(event.target, renderConstructor);
  });
  cookElements.constructorFieldDialog.addEventListener("change", event => {
    handleFieldEditorInput(event.target);
  });
}
