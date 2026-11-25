import type { SecurityCertificate } from "../../types";
import { authorityCandidates } from "./wizard-authority-data";
import { type WizardCertificateStatus, apiCertificateStatuses, defaultAPICertificateSelection, identityStatuses } from "./wizard-certificate-data";
import { type NodeClusterPlan, type WizardConfigNode, clusterPlans } from "./wizard-config-data";
import { healthStatuses, profileStatuses } from "./wizard-health-data";
import { type WizardNodeReport, masterReport } from "./wizard-nodes-data";
import type { WizardNodeStatus } from "./wizard-status-data";
import { tokenStatuses, trustStatuses } from "./wizard-token-trust-data";

export interface WizardPlanInput {
  reports: readonly WizardNodeReport[];
  authorityID: string;
  apiSelection: Readonly<Record<string, boolean>>;
  clusterPorts: Readonly<Record<string, string>>;
  now: Date;
}

export interface WizardPlan {
  candidates: SecurityCertificate[];
  authority: SecurityCertificate | undefined;
  token: WizardNodeStatus[];
  identity: WizardCertificateStatus[];
  apiCertificate: WizardCertificateStatus[];
  trust: WizardNodeStatus[];
  config: NodeClusterPlan[];
  profile: WizardNodeStatus[];
  health: WizardNodeStatus[];
}

export function wizardPlan(input: WizardPlanInput): WizardPlan {
  const candidates = authorityCandidates(masterReport(input.reports)?.summary?.certificates ?? []);
  const authority = candidates.find(candidate => candidate.id === input.authorityID);
  return {
    candidates,
    authority,
    token: tokenStatuses(input.reports),
    identity: identityStatuses(input.reports, authority, input.now),
    apiCertificate: apiCertificateStatuses(input.reports, authority, report => apiCertificateSelected(input.apiSelection, report), input.now),
    trust: trustStatuses(input.reports, authority),
    config: clusterPlans(configNodes(input.reports), input.clusterPorts),
    profile: profileStatuses(input.reports),
    health: healthStatuses(input.reports)
  };
}

export function apiCertificateSelected(selection: Readonly<Record<string, boolean>>, report: WizardNodeReport): boolean {
  return selection[report.node.key] ?? defaultAPICertificateSelection(report.summary);
}

function configNodes(reports: readonly WizardNodeReport[]): WizardConfigNode[] {
  return reports.flatMap(report => report.summary
    ? [{key: report.node.key, label: report.node.label, role: report.node.role, nodeURL: report.node.nodeURL, cluster: report.summary.cluster}]
    : []);
}
