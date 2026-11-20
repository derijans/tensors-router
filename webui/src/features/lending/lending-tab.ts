import { SafeHTML, html, setHTML } from "../../lib/safe-html";
import { getLendingDecisions, getLendingSettings, getLendingSummary, resetAllLendingSettings, resetLendingSetting, saveLendingSettings } from "../../api/lending-api";
import { type LendingDecisionGroup, collapseDecisions } from "./lending-log-collapse-data";
import { decisionMatchesSearch, renderDecisionDetail, renderDecisionRows } from "./lending-log-view";
import { pendingSettingChanges, renderLendingSettingRows } from "./lending-settings-view";
import { renderLendingSummary } from "./lending-summary-view";
import { reportErrorToConsole } from "../../api/console-report";
import { bindGuardianPanel, loadGuardians } from "../guardian/guardian-panel";
import { elementTarget } from "../../lib/dom";
import type { LendingDecision, LendingNodeError, LendingSettingsResponse, LendingSummaryResponse } from "../../types";
import { errorMessage } from "../../lib/errors";
import { errorText, mutedText, nodeErrorText } from "../../ui/text";
import { lendingElements } from "./lending-elements";

const pollIntervalMilliseconds = 5000;

type TaskRunner = (task: () => Promise<void>, key: string, label: string) => void;

interface LendingView {
  settings: LendingSettingsResponse | null;
  summary: LendingSummaryResponse | null;
  decisions: LendingDecision[];
  nodeErrors: LendingNodeError[];
  selectedGroupID: string;
  error: string;
  pollTimer: number | null;
}

const view: LendingView = {
  settings: null,
  summary: null,
  decisions: [],
  nodeErrors: [],
  selectedGroupID: "",
  error: "",
  pollTimer: null
};

export async function loadLending(): Promise<void> {
  await Promise.all([refresh(true), loadGuardians()]);
}

export function setLendingTabActive(active: boolean): void {
  if (view.pollTimer !== null) {
    window.clearInterval(view.pollTimer);
    view.pollTimer = null;
  }
  if (active) {
    view.pollTimer = window.setInterval(() => void refreshKeepingTypedSettings(), pollIntervalMilliseconds);
  }
}

export function bindLendingTab(run: TaskRunner): void {
  const reload = (): void => run(loadLending, "lending-refresh", "Loading lending…");
  bindGuardianPanel(run);
  lendingElements.lendingRefreshButton.addEventListener("click", reload);
  for (const filter of [lendingElements.lendingWindowSelect, lendingElements.lendingLaneSelect, lendingElements.lendingOutcomeSelect]) {
    filter.addEventListener("change", () => run(refreshKeepingTypedSettings, "lending-filter", "Loading lending…"));
  }
  lendingElements.lendingSearchInput.addEventListener("input", renderDecisions);
  lendingElements.lendingSettingsSaveButton.addEventListener("click", () => run(saveEditedSettings, "lending-save", "Saving lending settings…"));
  lendingElements.lendingSettingsResetAllButton.addEventListener("click", () => run(() => changeSettingsAndReloadNodes(resetAllLendingSettings()), "lending-reset", "Resetting lending settings…"));
  lendingElements.lendingSettingsRows.addEventListener("click", event => {
    const key = elementTarget(event)?.closest<HTMLElement>("[data-lending-setting-reset]")?.dataset.lendingSettingReset;
    if (key) {
      run(() => changeSettingsAndReloadNodes(resetLendingSetting(key)), "lending-reset", "Resetting lending setting…");
    }
  });
  lendingElements.lendingDecisionRows.addEventListener("click", event => {
    const groupID = elementTarget(event)?.closest<HTMLElement>("[data-lending-group]")?.dataset.lendingGroup;
    if (groupID) {
      view.selectedGroupID = view.selectedGroupID === groupID ? "" : groupID;
      renderDecisions();
    }
  });
}

function refreshKeepingTypedSettings(): Promise<void> {
  return refresh(false);
}

async function refresh(includeSettings: boolean): Promise<void> {
  try {
    const [settings, summary, decisions] = await Promise.all([
      includeSettings || !view.settings ? getLendingSettings() : Promise.resolve(view.settings),
      getLendingSummary(),
      getLendingDecisions({
        sinceMilliseconds: sinceMilliseconds(lendingElements.lendingWindowSelect.value),
        lane: lendingElements.lendingLaneSelect.value,
        outcome: lendingElements.lendingOutcomeSelect.value
      })
    ]);
    view.settings = settings;
    view.summary = summary;
    view.decisions = decisions.records;
    view.nodeErrors = distinctNodeErrors([...(summary.node_errors ?? []), ...(decisions.node_errors ?? [])]);
    view.error = "";
  } catch (error) {
    view.error = errorMessage(error);
    reportErrorToConsole("lending panel", error);
  }
  render(includeSettings);
}

async function saveEditedSettings(): Promise<void> {
  if (!view.settings) {
    return;
  }
  const inputs = [...lendingElements.lendingSettingsRows.querySelectorAll<HTMLInputElement | HTMLSelectElement>("[data-lending-setting-input]")]
    .map(input => ({key: input.dataset.lendingSettingInput ?? "", value: input.value}));
  const changes = pendingSettingChanges(view.settings.entries, inputs);
  if (Object.keys(changes.set).length > 0) {
    await saveLendingSettings(changes.set);
  }
  for (const key of changes.reset) {
    await resetLendingSetting(key);
  }
  await refresh(true);
}

async function changeSettingsAndReloadNodes(change: Promise<LendingSettingsResponse>): Promise<void> {
  await change;
  await refresh(true);
}

function distinctNodeErrors(errors: LendingNodeError[]): LendingNodeError[] {
  const seen = new Set<string>();
  return errors.filter(error => {
    const key = `${error.node_id}\u0000${error.error}`;
    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
}

function sinceMilliseconds(windowValue: string): number | null {
  const window = Number(windowValue);
  return windowValue === "" || !Number.isFinite(window) ? null : Date.now() - window;
}

function render(includeSettings: boolean): void {
  const notices: SafeHTML[] = [];
  if (view.error) {
    notices.push(errorText(view.error));
  }
  for (const nodeError of view.nodeErrors) {
    notices.push(nodeErrorText(nodeError));
  }
  if (view.settings && !view.settings.editable) {
    notices.push(mutedText("This router has no settings store; values come from defaults and the config file only.", "p"));
  }
  setHTML(lendingElements.lendingStatus, html`${notices}`);
  if (view.summary) {
    setHTML(lendingElements.lendingSummary, renderLendingSummary(view.summary, view.settings?.fingerprint ?? "", new Date()));
  }
  if (includeSettings && view.settings) {
    setHTML(lendingElements.lendingSettingsRows, renderLendingSettingRows(view.settings.entries, view.settings.editable));
  }
  renderDecisions();
}

function renderDecisions(): void {
  const search = lendingElements.lendingSearchInput.value;
  const groups = collapseDecisions(view.decisions.filter(decision => decisionMatchesSearch(decision, search)));
  setHTML(lendingElements.lendingDecisionRows, renderDecisionRows(groups, view.selectedGroupID));
  setHTML(lendingElements.lendingDecisionDetail, renderDecisionDetail(selectedGroup(groups)));
}

function selectedGroup(groups: LendingDecisionGroup[]): LendingDecisionGroup | undefined {
  return groups.find(group => group.id === view.selectedGroupID);
}
