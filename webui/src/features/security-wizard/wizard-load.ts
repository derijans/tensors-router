import { getSecuritySummary } from "../../api/security-target-api";
import { errorMessage } from "../../lib/errors";
import { state } from "../../state/app-state";
import { loadSecurity } from "../security/security";
import { preferredAuthorityID } from "./wizard-authority-data";
import { type WizardNodeReport, masterReport, wizardNodes } from "./wizard-nodes-data";
import { renderWizard } from "./wizard-render";
import { wizardState } from "./wizard-state";

let latestLoad = 0;

export async function loadWizard(): Promise<void> {
  const load = ++latestLoad;
  const nodes = wizardNodes(state.inventory);
  wizardState.loading = true;
  wizardState.error = "";
  renderWizard();
  const settled = await Promise.allSettled(nodes.map(node => getSecuritySummary(node.target)));
  if (load !== latestLoad) {
    return;
  }
  wizardState.reports = nodes.map((node, index): WizardNodeReport => {
    const result = settled[index];
    return result?.status === "fulfilled"
      ? {node, summary: result.value, error: ""}
      : {node, summary: null, error: result ? errorMessage(result.reason) : ""};
  });
  wizardState.authorityID = preferredAuthorityID(masterReport(wizardState.reports)?.summary?.certificates ?? [], wizardState.authorityID);
  wizardState.loading = false;
  renderWizard();
}

export async function openWizard(): Promise<void> {
  wizardState.open = true;
  await loadWizard();
}

export function closeWizard(): void {
  wizardState.open = false;
  renderWizard();
}

export async function reloadAfterWizardChange(): Promise<void> {
  await Promise.all([loadWizard(), loadSecurity()]);
}
