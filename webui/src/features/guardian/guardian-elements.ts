import { getRequiredElement } from "../../lib/dom";

export const guardianElements = {
  guardianStatus: getRequiredElement("guardianStatus", HTMLElement),
  guardianRows: getRequiredElement("guardianRows", HTMLTableSectionElement)
};
