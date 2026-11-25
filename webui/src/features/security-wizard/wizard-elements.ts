import { getRequiredElement } from "../../lib/dom";

export const wizardElements = {
  openButton: getRequiredElement("securityWizardButton", HTMLButtonElement),
  card: getRequiredElement("securityWizard", HTMLElement),
  refreshButton: getRequiredElement("securityWizardRefresh", HTMLButtonElement),
  closeButton: getRequiredElement("securityWizardClose", HTMLButtonElement),
  status: getRequiredElement("securityWizardStatus", HTMLElement),
  steps: getRequiredElement("securityWizardSteps", HTMLElement)
};
