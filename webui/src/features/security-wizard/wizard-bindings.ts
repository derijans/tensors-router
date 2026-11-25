import { elementTarget } from "../../lib/dom";
import { runTask } from "../shell/tasks";
import { securityGroup } from "../security/security-dialog-bindings";
import { createWizardAuthority, runCertificateStep, runProfileStep, runTokenStep, runTrustStep } from "./wizard-actions";
import { wizardElements } from "./wizard-elements";
import { closeWizard, loadWizard, openWizard } from "./wizard-load";
import { renderWizard } from "./wizard-render";
import { downloadWizardRequest, installWizardSignedCertificate } from "./wizard-request-actions";
import { wizardState } from "./wizard-state";

const stepRunners: Record<string, () => Promise<void>> = {
  token: runTokenStep,
  identity: () => runCertificateStep("identity", "identity"),
  apiCertificate: () => runCertificateStep("apiCertificate", "api_certificate"),
  trust: runTrustStep,
  profile: runProfileStep
};

export function bindSecurityWizard(): void {
  wizardElements.openButton.addEventListener("click", () => {
    runTask(openWizard, "security-wizard-load", securityGroup, "Reading routers…");
  });
  wizardElements.refreshButton.addEventListener("click", () => {
    runTask(loadWizard, "security-wizard-load", securityGroup, "Reading routers…");
  });
  wizardElements.closeButton.addEventListener("click", closeWizard);
  wizardElements.steps.addEventListener("click", handleStepClick);
  wizardElements.steps.addEventListener("change", handleStepChange);
}

function handleStepClick(event: Event): void {
  const data = elementTarget(event)?.closest<HTMLElement>("button")?.dataset ?? {};
  const runner = data.wizardRun ? stepRunners[data.wizardRun] : undefined;
  if (data.wizardRun && runner) {
    runTask(runner, `security-wizard-${data.wizardRun}`, securityGroup, "Running setup step…");
  } else if (data.wizardCreateAuthority !== undefined) {
    runTask(createWizardAuthority, "security-wizard-authority", securityGroup, "Creating CA…");
  } else if (data.wizardRequestDownload && data.wizardNode !== undefined) {
    downloadWizardRequest(data.wizardNode, data.wizardRequestDownload);
  } else if (data.wizardInstallSigned !== undefined) {
    const nodeKey = data.wizardInstallSigned;
    runTask(() => installWizardSignedCertificate(nodeKey), "security-wizard-install", securityGroup, "Loading router…");
  }
}

function handleStepChange(event: Event): void {
  const control = elementTarget(event);
  if ((control instanceof HTMLInputElement || control instanceof HTMLSelectElement) && applyStepChange(control)) {
    renderWizard();
  }
}

function applyStepChange(control: HTMLInputElement | HTMLSelectElement): boolean {
  const {wizardAuthority, wizardAuthorityName, wizardApiNode, wizardClusterPort} = control.dataset;
  if (wizardAuthorityName !== undefined) {
    wizardState.newAuthorityName = control.value;
    return false;
  }
  if (wizardAuthority !== undefined) {
    wizardState.authorityID = control.value;
    return true;
  }
  if (wizardApiNode !== undefined && control instanceof HTMLInputElement) {
    wizardState.apiSelection[wizardApiNode] = control.checked;
    return true;
  }
  if (wizardClusterPort !== undefined) {
    wizardState.clusterPorts[wizardClusterPort] = control.value.trim();
    return true;
  }
  return false;
}
