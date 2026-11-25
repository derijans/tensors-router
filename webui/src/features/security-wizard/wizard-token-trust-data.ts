import type { SecurityCertificate } from "../../types";
import { holdsAuthority } from "./wizard-authority-data";
import type { WizardNodeReport } from "./wizard-nodes-data";
import { type WizardNodeStatus, nodeStatus, unreachableStatus } from "./wizard-status-data";

export function tokenStatuses(reports: readonly WizardNodeReport[]): WizardNodeStatus[] {
  return reports.map(tokenStatus);
}

function tokenStatus(report: WizardNodeReport): WizardNodeStatus {
  if (!report.summary) {
    return unreachableStatus(report);
  }
  const clusterToken = report.summary.tokens.find(token => token.role === "cluster");
  if (report.node.role === "slave") {
    return nodeStatus(report, "done", "Accepts the master's cluster token, because the master reads its summary with it.");
  }
  return clusterToken
    ? nodeStatus(report, "done", `Holds cluster token ${clusterToken.name}.`)
    : nodeStatus(report, "todo", "Creates a cluster token on the master.");
}

export function trustStatuses(reports: readonly WizardNodeReport[], authority: SecurityCertificate | undefined): WizardNodeStatus[] {
  return reports.map(report => trustStatus(report, authority));
}

function trustStatus(report: WizardNodeReport, authority: SecurityCertificate | undefined): WizardNodeStatus {
  if (!report.summary) {
    return unreachableStatus(report);
  }
  if (!authority) {
    return nodeStatus(report, "blocked", "Choose a CA in step 2.");
  }
  if (report.node.role === "master") {
    return nodeStatus(report, "done", `Holds ${authority.name} and trusts the identities it signs.`);
  }
  return holdsAuthority(report.summary.certificates, authority)
    ? nodeStatus(report, "done", `Trusts ${authority.name}.`)
    : nodeStatus(report, "todo", `Installs ${authority.name} as Trusted CA.`);
}
