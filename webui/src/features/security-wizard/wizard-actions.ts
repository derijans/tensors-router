import { createSecurityAuthority } from "../../api/security-api";
import { trustMasterAuthority } from "../../api/security-node-api";
import { createSecurityCertificateRequest, createSecurityToken } from "../../api/security-target-api";
import { issueFromAuthority, switchProfileToSecure } from "../../api/security-wizard-api";
import { errorMessage } from "../../lib/errors";
import type { SecurityCertificate, SecurityRequestKind } from "../../types";
import { confirmDestructive } from "../shell/dialogs";
import { signsOnMaster } from "./wizard-authority-data";
import { type WizardNodeReport, availableName, credentialNames } from "./wizard-nodes-data";
import { currentWizardPlan } from "./wizard-render";
import { reloadAfterWizardChange } from "./wizard-load";
import { type WizardStep, type WizardStepResults, wizardState } from "./wizard-state";
import type { WizardNodeStatus } from "./wizard-status-data";

type NodeWork = (report: WizardNodeReport) => Promise<string>;

const certificateNameSuffixes: Record<SecurityRequestKind, string> = {
  identity: "identity",
  api_certificate: "api"
};

const secureProfileConsequences = "Each listed router rewrites security.profile in its config.yaml. After a restart, model APIs need an inference token and admin APIs need an admin token. Set router.token in webui.yaml to an admin token before you restart the master.";

export async function runTokenStep(): Promise<void> {
  await runForNodes("token", currentWizardPlan().token, createMasterClusterToken);
}

export async function runCertificateStep(step: "identity" | "apiCertificate", kind: SecurityRequestKind): Promise<void> {
  const plan = currentWizardPlan();
  const authority = plan.authority;
  if (!authority) {
    throw new Error("Choose a CA in step 2 first.");
  }
  await runForNodes(step, plan[step], report => issueOrRequest(report, kind, authority));
}

export async function runTrustStep(): Promise<void> {
  const authority = currentWizardPlan().authority;
  if (!authority) {
    throw new Error("Choose a CA in step 2 first.");
  }
  await runForNodes("trust", currentWizardPlan().trust, async report => {
    await trustMasterAuthority(report.node.key, authority.id);
    return `Installed ${authority.name} as Trusted CA.`;
  });
}

export async function runProfileStep(): Promise<void> {
  const labels = currentWizardPlan().profile.filter(status => status.state === "todo").map(status => status.label);
  if (!await confirmDestructive(`Switch ${labels.join(", ")} to secure?`, secureProfileConsequences, "Switch to secure")) {
    return;
  }
  await runForNodes("profile", currentWizardPlan().profile, async report => {
    await switchProfileToSecure(report.node.target);
    return "Updated config.yaml; restart the router to apply the secure profile.";
  });
}

export async function createWizardAuthority(): Promise<void> {
  const name = wizardState.newAuthorityName.trim();
  if (name === "") {
    throw new Error("Enter a name for the new CA.");
  }
  await recordResults("authority", async () => {
    const authority = await createSecurityAuthority({name});
    wizardState.authorityID = authority.id;
    return {"": {ok: true, message: `Created local CA ${authority.name}.`}};
  });
}

async function createMasterClusterToken(report: WizardNodeReport): Promise<string> {
  const taken = report.summary?.tokens.map(token => token.name) ?? [];
  const created = await createSecurityToken(report.node.target, {name: availableName("cluster", taken), role: "cluster", value: ""});
  return `Created cluster token ${created.token.name}.`;
}

async function issueOrRequest(report: WizardNodeReport, kind: SecurityRequestKind, authority: SecurityCertificate): Promise<string> {
  const summary = report.summary;
  if (!summary) {
    throw new Error(report.error || "The router is unreachable.");
  }
  const request = {
    name: availableName(`${summary.node_id || report.node.label}-${certificateNameSuffixes[kind]}`, credentialNames(summary)),
    kind,
    hosts: summary.default_hosts
  };
  if (!signsOnMaster(authority)) {
    await createSecurityCertificateRequest(report.node.target, request);
    return `Created request ${request.name}. Download it, sign it with ${authority.name}, then install the signed certificate.`;
  }
  await issueFromAuthority(report.node.target, {...request, authority_id: authority.id, activate: true});
  return `Installed and activated ${request.name}.`;
}

async function runForNodes(step: WizardStep, statuses: readonly WizardNodeStatus[], work: NodeWork): Promise<void> {
  const pending = statuses.filter(status => status.state === "todo");
  await recordResults(step, async () => {
    const results: WizardStepResults = {};
    for (const status of pending) {
      const report = wizardState.reports.find(candidate => candidate.node.key === status.nodeKey);
      if (report) {
        results[status.nodeKey] = await settledResult(() => work(report));
      }
    }
    return results;
  });
}

async function settledResult(work: () => Promise<string>): Promise<{ok: boolean; message: string}> {
  try {
    return {ok: true, message: await work()};
  } catch (error) {
    return {ok: false, message: errorMessage(error)};
  }
}

async function recordResults(step: WizardStep, run: () => Promise<WizardStepResults>): Promise<void> {
  try {
    wizardState.results[step] = await run();
  } finally {
    await reloadAfterWizardChange();
  }
}
