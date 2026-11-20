import { setHTML, html } from "../../lib/safe-html";
import { clearGuardianModel, getGuardianModels, saveGuardianModel } from "../../api/guardian-api";
import { guardianAssignment, guardianRows, guardianTargets, parseEndpointValue, type GuardianTarget } from "./guardian-data";
import { renderGuardianRows } from "./guardian-view";
import { elementTarget } from "../../lib/dom";
import { reportErrorToConsole } from "../../api/console-report";
import { state } from "../../state/app-state";
import type { GuardianModelsResponse, GuardianScope } from "../../types";
import { errorMessage } from "../../lib/errors";
import { errorText } from "../../ui/text";
import { guardianElements } from "./guardian-elements";

type TaskRunner = (task: () => Promise<void>, key: string, label: string) => void;

let guardians: GuardianModelsResponse | null = null;
let failure = "";

export async function loadGuardians(): Promise<void> {
  try {
    guardians = await getGuardianModels();
    failure = "";
  } catch (error) {
    failure = errorMessage(error);
    reportErrorToConsole("guardian models", error);
  }
  renderGuardians();
}

export function bindGuardianPanel(run: TaskRunner): void {
  guardianElements.guardianRows.addEventListener("change", event => {
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
    failure = errorMessage(error);
  }
  renderGuardians();
}

function renderGuardians(): void {
  setHTML(guardianElements.guardianStatus, failure === "" ? html`` : errorText(failure));
  if (guardians === null) {
    return;
  }
  const rows = guardianRows(guardianTargets(state.inventory), guardians.assignments);
  setHTML(guardianElements.guardianRows, renderGuardianRows(rows, guardians.candidates, true));
}
