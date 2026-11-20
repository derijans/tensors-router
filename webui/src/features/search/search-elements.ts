import { getRequiredElement } from "../../lib/dom";

export const searchElements = {
  globalSearchInput: getRequiredElement("globalSearchInput", HTMLInputElement),
  globalSearchResults: getRequiredElement("globalSearchResults", HTMLUListElement)
};
