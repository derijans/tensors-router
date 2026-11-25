import { SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import { attributes } from "../../ui/attributes";
import { emptyState } from "../../ui/empty-state";
import type { NodeClusterPlan } from "./wizard-config-data";
import type { WizardNodeReport } from "./wizard-nodes-data";

export function configPlansMarkup(plans: readonly NodeClusterPlan[]): SafeHTML {
  return plans.length === 0
    ? emptyState("No router summary is loaded yet.")
    : html`<div class="security-wizard-configs">${plans.map(configPlanMarkup)}</div>`;
}

function configPlanMarkup(plan: NodeClusterPlan): SafeHTML {
  return html`
    <section class="security-wizard-config">
      <header class="security-wizard-step-head">
        <h4>${plan.label}</h4>
        <label>Cluster port<input type="text" inputmode="numeric"${attributes({"data-wizard-cluster-port": plan.nodeKey, value: plan.port})}></label>
      </header>
      ${plan.problem ? html`<p class="security-notice tone-warning">${plan.problem}</p>` : emptyHTML}
      ${plan.changes.length === 0 ? html`<p class="muted">No change needed.</p>` : changesMarkup(plan)}
      <pre class="security-wizard-yaml">${plan.yaml}</pre>
    </section>
  `;
}

function changesMarkup(plan: NodeClusterPlan): SafeHTML {
  return html`
    <table class="security-table">
      <thead><tr><th>Setting</th><th>Current</th><th>New</th></tr></thead>
      <tbody>${plan.changes.map(change => html`<tr><td><code>cluster.${change.setting}</code></td><td class="muted">${change.current || "(empty)"}</td><td><code>${change.proposed}</code></td></tr>`)}</tbody>
    </table>
  `;
}

export function restartStepMarkup(order: readonly WizardNodeReport[]): SafeHTML {
  return html`
    <ol class="security-wizard-restart">
      ${order.map(report => html`<li>${restartInstruction(report)}</li>`)}
    </ol>
  `;
}

function restartInstruction(report: WizardNodeReport): string {
  if (report.node.role === "master") {
    return `Restart ${report.node.label} last, after every slave is back. A router managed by this WebUI restarts with Restart in the top bar; otherwise restart it on its host.`;
  }
  return `Restart ${report.node.label} on its own host: stop it (for example POST /router/v1/shutdown with an admin token, or stop its service), then start it again with its config.yaml. The master cannot restart a slave.`;
}
