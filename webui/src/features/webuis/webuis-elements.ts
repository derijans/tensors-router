import { getRequiredElement } from "../../lib/dom";

export const webuisElements = {
  webuiFilterInput: getRequiredElement("webuiFilterInput", HTMLInputElement),
  webuiStatus: getRequiredElement("webuiStatus", HTMLElement),
  webuiGrid: getRequiredElement("webuiGrid", HTMLElement),
  webuiDialog: getRequiredElement("webuiDialog", HTMLDialogElement),
  webuiDialogBody: getRequiredElement("webuiDialogBody", HTMLElement)
};
