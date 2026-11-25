import { setHTML } from "../../lib/safe-html";
import { state } from "../../state/app-state";
import { showStatus } from "../../ui/status-line";
import { wizardElements } from "./wizard-elements";
import { wizardNodes } from "./wizard-nodes-data";
import { type WizardPlan, apiCertificateSelected, wizardPlan } from "./wizard-plan-data";
import { wizardState } from "./wizard-state";
import { wizardStepsMarkup } from "./wizard-view";

export function currentWizardPlan(): WizardPlan {
  return wizardPlan({
    reports: wizardState.reports,
    authorityID: wizardState.authorityID,
    apiSelection: wizardState.apiSelection,
    clusterPorts: wizardState.clusterPorts,
    now: new Date()
  });
}

export function renderWizardEntry(): void {
  const available = wizardNodes(state.inventory).length > 0;
  wizardElements.openButton.hidden = !available;
  if (!available && wizardState.open) {
    wizardState.open = false;
    renderWizard();
  }
}

export function renderWizard(): void {
  wizardElements.card.hidden = !wizardState.open;
  if (!wizardState.open) {
    return;
  }
  showStatus(wizardElements.status, wizardState.loading ? "Reading every router's security summary…" : wizardState.error, wizardState.error !== "");
  setHTML(wizardElements.steps, wizardStepsMarkup({
    plan: currentWizardPlan(),
    reports: wizardState.reports,
    results: wizardState.results,
    newAuthorityName: wizardState.newAuthorityName,
    apiCertificateSelected: nodeKey => {
      const report = wizardState.reports.find(candidate => candidate.node.key === nodeKey);
      return report ? apiCertificateSelected(wizardState.apiSelection, report) : false;
    }
  }));
}
