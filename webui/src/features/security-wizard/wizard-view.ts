import { SafeHTML, html } from "../../lib/safe-html";
import { configPlansMarkup, restartStepMarkup } from "./wizard-config-view";
import { apiCertificateChoiceMarkup, authorityControlsMarkup, pendingRequestActions } from "./wizard-controls-view";
import { restartOrder } from "./wizard-health-data";
import type { WizardNodeReport } from "./wizard-nodes-data";
import type { WizardPlan } from "./wizard-plan-data";
import type { WizardStep, WizardStepResults } from "./wizard-state";
import { type WizardNodeStatus, nodeStatus } from "./wizard-status-data";
import { wizardStepMarkup } from "./wizard-step-view";

export interface WizardViewInput {
  plan: WizardPlan;
  reports: readonly WizardNodeReport[];
  results: Partial<Record<WizardStep, WizardStepResults>>;
  newAuthorityName: string;
  apiCertificateSelected: (nodeKey: string) => boolean;
}

export function wizardStepsMarkup(input: WizardViewInput): SafeHTML {
  const {plan, results} = input;
  return html`
    ${wizardStepMarkup({
      run: "token",
      number: 1,
      title: "Cluster token",
      description: "Every router needs the master's cluster token. A slave the master can reach already holds it; give an unreachable slave the token by hand once (Create token, Existing value).",
      statuses: plan.token,
      results: results.token
    })}
    ${wizardStepMarkup({
      run: "",
      number: 2,
      title: "Certificate authority",
      description: "Pick the CA that signs every router identity: a local CA on the master, or a trusted CA such as the one from pfSense. Or create a new local CA.",
      statuses: authorityStatuses(input),
      results: results.authority,
      controls: authorityControlsMarkup(plan.candidates, plan.authority, input.newAuthorityName)
    })}
    ${wizardStepMarkup({
      run: "identity",
      number: 3,
      title: "Router identities",
      description: "Each router creates its key and a request, the CA signs it, and the router installs and activates the identity. Private keys never leave a router.",
      statuses: plan.identity,
      results: results.identity,
      rowActions: pendingRequestActions
    })}
    ${wizardStepMarkup({
      run: "apiCertificate",
      number: 4,
      title: "API certificates",
      description: "The certificate API clients and browsers see. Keep one imported from config or the WebUI files, or tick Issue to get one from the CA.",
      statuses: plan.apiCertificate,
      results: results.apiCertificate,
      rowActions: status => [apiCertificateChoiceMarkup(status, input.apiCertificateSelected(status.nodeKey)), ...pendingRequestActions(status)]
    })}
    ${wizardStepMarkup({
      run: "trust",
      number: 5,
      title: "Trust",
      description: "Every slave installs the CA as Trusted CA, so it accepts the identities the CA signs. The master sends only the CA certificate.",
      statuses: plan.trust,
      results: results.trust
    })}
    ${wizardStepMarkup({
      run: "profile",
      number: 6,
      title: "Configuration",
      description: "Give cluster peers their own HTTPS port: put each block in that router's config.yaml under cluster, because the WebUI does not edit these settings. Then switch the routers to the secure profile, so model and admin APIs require tokens; the switch rewrites security.profile and applies after a restart.",
      statuses: plan.profile,
      results: results.profile,
      controls: configPlansMarkup(plan.config)
    })}
    ${wizardStepMarkup({
      run: "",
      number: 7,
      title: "Restart and health",
      description: restartStepMarkup(restartOrder(input.reports)),
      statuses: plan.health,
      results: undefined
    })}
  `;
}

function authorityStatuses(input: WizardViewInput): WizardNodeStatus[] {
  const master = input.reports.find(report => report.node.role === "master");
  if (!master) {
    return [];
  }
  return [input.plan.authority
    ? nodeStatus(master, "done", `${input.plan.authority.name} is selected.`)
    : nodeStatus(master, "blocked", "Create a local CA or import a trusted CA on the master.")];
}
