import type { Tone } from "../../types";
import type { WizardNodeReport } from "./wizard-nodes-data";

export type WizardNodeState = "done" | "todo" | "waiting" | "blocked" | "skipped" | "unreachable";

export interface WizardNodeStatus {
  nodeKey: string;
  label: string;
  state: WizardNodeState;
  detail: string;
}

const stateLabels: Record<WizardNodeState, string> = {
  done: "Done",
  todo: "To do",
  waiting: "Waiting",
  blocked: "Blocked",
  skipped: "Skipped",
  unreachable: "Unreachable"
};

const stateTones: Record<WizardNodeState, Tone> = {
  done: "success",
  todo: "accent",
  waiting: "warning",
  blocked: "danger",
  skipped: "neutral",
  unreachable: "danger"
};

export function stateLabel(state: WizardNodeState): string {
  return stateLabels[state];
}

export function stateTone(state: WizardNodeState): Tone {
  return stateTones[state];
}

export function nodeStatus(report: WizardNodeReport, state: WizardNodeState, detail: string): WizardNodeStatus {
  return {nodeKey: report.node.key, label: report.node.label, state, detail};
}

export function unreachableStatus(report: WizardNodeReport): WizardNodeStatus {
  return nodeStatus(report, "unreachable", report.error || "The master cannot read this router's security summary.");
}

export function stepDone(statuses: readonly WizardNodeStatus[]): boolean {
  return statuses.length > 0 && statuses.every(status => status.state === "done" || status.state === "skipped");
}

export function hasWork(statuses: readonly WizardNodeStatus[]): boolean {
  return statuses.some(status => status.state === "todo");
}
