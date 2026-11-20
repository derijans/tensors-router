import { getRequiredElement } from "../../lib/dom";

export const lendingElements = {
  lendingRefreshButton: getRequiredElement("lendingRefreshButton", HTMLButtonElement),
  lendingStatus: getRequiredElement("lendingStatus", HTMLElement),
  lendingSummary: getRequiredElement("lendingSummary", HTMLElement),
  lendingSettingsSaveButton: getRequiredElement("lendingSettingsSaveButton", HTMLButtonElement),
  lendingSettingsResetAllButton: getRequiredElement("lendingSettingsResetAllButton", HTMLButtonElement),
  lendingSettingsRows: getRequiredElement("lendingSettingsRows", HTMLTableSectionElement),
  lendingWindowSelect: getRequiredElement("lendingWindowSelect", HTMLSelectElement),
  lendingLaneSelect: getRequiredElement("lendingLaneSelect", HTMLSelectElement),
  lendingOutcomeSelect: getRequiredElement("lendingOutcomeSelect", HTMLSelectElement),
  lendingSearchInput: getRequiredElement("lendingSearchInput", HTMLInputElement),
  lendingDecisionRows: getRequiredElement("lendingDecisionRows", HTMLTableSectionElement),
  lendingDecisionDetail: getRequiredElement("lendingDecisionDetail", HTMLElement)
};
