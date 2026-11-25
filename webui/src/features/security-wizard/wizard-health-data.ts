import type { SecurityListener, SecurityStatus } from "../../types";
import type { WizardNodeReport } from "./wizard-nodes-data";
import { type WizardNodeStatus, nodeStatus, unreachableStatus } from "./wizard-status-data";

const listenerLabels: Record<SecurityListener["name"], string> = {
  api: "API port",
  cluster: "Cluster port",
  shared: "Port"
};

export function profileStatuses(reports: readonly WizardNodeReport[]): WizardNodeStatus[] {
  return reports.map(report => {
    const status = report.summary?.status;
    if (!status) {
      return unreachableStatus(report);
    }
    if (status.profile_restart_required) {
      return nodeStatus(report, "waiting", "config.yaml sets the secure profile; it applies after a restart.");
    }
    if (status.profile === "secure") {
      return nodeStatus(report, "done", "Runs the secure profile.");
    }
    if (status.secure_profile_blocker) {
      return nodeStatus(report, "blocked", `Before switching to secure: ${status.secure_profile_blocker}.`);
    }
    return nodeStatus(report, "todo", "Runs trusted_lan: model and admin APIs accept requests without a token.");
  });
}

export function restartOrder(reports: readonly WizardNodeReport[]): WizardNodeReport[] {
  return [...reports.filter(report => report.node.role === "slave"), ...reports.filter(report => report.node.role === "master")];
}

export function healthStatuses(reports: readonly WizardNodeReport[]): WizardNodeStatus[] {
  return restartOrder(reports).map(report => {
    const status = report.summary?.status;
    if (!status) {
      return unreachableStatus(report);
    }
    const detail = [...status.listeners.map(listenerHealth), profileHealth(status)].join(" ");
    if (needsRestart(status)) {
      return nodeStatus(report, "waiting", `Restart needed. ${detail}`);
    }
    return nodeStatus(report, healthy(status) ? "done" : "todo", detail);
  });
}

function listenerHealth(listener: SecurityListener): string {
  const served = listener.restart_required ? "restart required" : listener.tls ? "HTTPS" : "plain HTTP";
  return `${listenerLabels[listener.name]} ${listener.bind}: ${served}.`;
}

function profileHealth(status: SecurityStatus): string {
  return status.profile_restart_required ? "Profile: secure after restart." : `Profile: ${status.profile}.`;
}

function needsRestart(status: SecurityStatus): boolean {
  return status.profile_restart_required || status.listeners.some(listener => listener.restart_required);
}

function healthy(status: SecurityStatus): boolean {
  return status.profile === "secure" && status.listeners.length > 0 && status.listeners.every(listener => listener.tls);
}
