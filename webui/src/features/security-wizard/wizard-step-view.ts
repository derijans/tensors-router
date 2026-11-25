import { type HTMLValue, SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import { badge } from "../../ui/badge";
import { buttonStrip, primaryButton } from "../../ui/buttons";
import { emptyTableRow } from "../../ui/empty-state";
import type { WizardRunResult, WizardStepResults } from "./wizard-state";
import { type WizardNodeStatus, hasWork, stateLabel, stateTone, stepDone } from "./wizard-status-data";

const nodeColumnCount = 4;

export interface WizardStepView<Status extends WizardNodeStatus = WizardNodeStatus> {
  run: string;
  number: number;
  title: string;
  description: HTMLValue;
  statuses: readonly Status[];
  results: WizardStepResults | undefined;
  controls?: SafeHTML;
  rowActions?: (status: Status) => SafeHTML[];
}

export function wizardStepMarkup<Status extends WizardNodeStatus>(view: WizardStepView<Status>): SafeHTML {
  const done = stepDone(view.statuses);
  const runButton = view.run && hasWork(view.statuses)
    ? primaryButton("Run step", {"data-wizard-run": view.run, "data-operation-group": "security"})
    : emptyHTML;
  return html`
    <li class="security-wizard-step">
      <header class="security-wizard-step-head">
        <h3>${view.number}. ${view.title}</h3>
        ${badge(done ? "Done" : "Open", done ? "success" : "neutral")}
        ${runButton}
      </header>
      <div class="security-wizard-description muted">${view.description}</div>
      ${view.controls ?? emptyHTML}
      <div class="table-wrap">
        <table class="security-table">
          <thead><tr><th>Router</th><th>State</th><th>Details</th><th></th></tr></thead>
          <tbody>${nodeRows(view)}</tbody>
        </table>
      </div>
    </li>
  `;
}

function nodeRows<Status extends WizardNodeStatus>(view: WizardStepView<Status>): SafeHTML {
  if (view.statuses.length === 0) {
    return emptyTableRow("No routers to show yet.", nodeColumnCount);
  }
  return html`${view.statuses.map(status => nodeRow(status, view.results?.[status.nodeKey], view.rowActions?.(status) ?? []))}`;
}

function nodeRow(status: WizardNodeStatus, result: WizardRunResult | undefined, actions: readonly SafeHTML[]): SafeHTML {
  return html`
    <tr>
      <td><strong>${status.label}</strong></td>
      <td>${badge(stateLabel(status.state), stateTone(status.state))}</td>
      <td>
        <div>${status.detail}</div>
        ${result ? html`<div class="security-wizard-result tone-${result.ok ? "success" : "danger"}">Last run: ${result.message}</div>` : emptyHTML}
      </td>
      <td class="security-actions">${buttonStrip(actions)}</td>
    </tr>
  `;
}
