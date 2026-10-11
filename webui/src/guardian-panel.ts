import { setHTML, html } from "./safe-html";
import { clearGuardianModel, getGuardianModels, saveGuardianModel } from "./guardian-api";
import { guardianAssignment, guardianRows, guardianTargets, parseEndpointValue, type GuardianTarget } from "./guardian-data";
import { renderGuardianRows } from "./guardian-view";
import { elements } from "./elements";
import { elementTarget } from "./dom";
import { reportErrorToConsole } from "./console-report";
import { state } from "./state";
import type { GuardianModelsResponse, GuardianScope } from "./types";

type TaskRunner = (task: () => Promise<void>, key: string, label: string) => void;

let guardians: GuardianModelsResponse | null = null;
let failure = "";

export async function loadGuardians(): Promise<void> {
  try {
    guardians = await getGuardianModels();
    failure = "";
  } catch (error) {
    failure = error instanceof Error ? error.message : String(error);
    reportErrorToConsole("guardian models", error);
  }
  renderGuardians();
}

export function bindGuardianPanel(run: TaskRunner): void {
  elements.guardianRows.addEventListener("change", event => {
    const select = elementTarget(event);
    const row = select?.closest<HTMLElement>("[data-guardian-scope]");
    if (!(select instanceof HTMLSelectElement) || !row) {
      return;
    }
    const target = targetOfRow(row);
    run(() => applyGuardianChoice(target, select.value), "guardian-save", "Saving guardian model…");
  });
}

function targetOfRow(row: HTMLElement): GuardianTarget {
  return {
    scope: row.dataset.guardianScope as GuardianScope,
    nodeID: row.dataset.guardianNode ?? "",
    modelID: row.dataset.guardianModel ?? "",
    label: ""
  };
}

async function applyGuardianChoice(target: GuardianTarget, chosen: string): Promise<void> {
  const guardian = parseEndpointValue(chosen);
  try {
    guardians = guardian === null
      ? await clearGuardianModel(target.scope, target.nodeID, target.modelID)
      : await saveGuardianModel(guardianAssignment(target, guardian));
    failure = "";
  } catch (error) {
    failure = error instanceof Error ? error.message : String(error);
  }
  renderGuardians();
}

function renderGuardians(): void {
  setHTML(elements.guardianStatus, failure === "" ? html`` : html`<p class="error-text">${failure}</p>`);
  if (guardians === null) {
    return;
  }
  const rows = guardianRows(guardianTargets(state.inventory), guardians.assignments);
  setHTML(elements.guardianRows, renderGuardianRows(rows, guardians.candidates, true));
}
