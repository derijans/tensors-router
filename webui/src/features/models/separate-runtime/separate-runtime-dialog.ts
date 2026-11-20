import { SafeHTML, emptyHTML, html, setHTML } from "../../../lib/safe-html";
import { fetchSeparateRuntime, saveSeparateRuntime } from "../../../api/models-api";
import {
  changedTriggerSelection,
  doNotUnloadSelected,
  groupTriggersByKind,
  selectedTriggerKeys,
  triggersFromSelection
} from "./separate-runtime-data";
import type { SeparateRuntimeCandidates } from "../../../types";
import { errorMessage } from "../../../lib/errors";
import { unloadPolicyLabels } from "../../../domain/unload-policy";
import { button, dialogActions, primaryButton } from "../../../ui/buttons";
import { modelsElements } from "../models-elements";
import { showStatus } from "../../../ui/status-line";
import { checkbox } from "../../../ui/form";

interface DialogSession {
  nodeId: string;
  localId: string;
  runSeparate: boolean;
  selected: Set<string>;
  candidates: SeparateRuntimeCandidates;
}

let session: DialogSession | null = null;

export function registerSeparateRuntimeDialog(refreshInventory: () => Promise<void>): void {
  modelsElements.separateRuntimeDialog.addEventListener("cancel", event => {
    event.preventDefault();
    closeDialog();
  });
  modelsElements.separateRuntimeDialog.addEventListener("change", event => {
    const target = event.target;
    if (!(target instanceof HTMLInputElement) || !session) {
      return;
    }
    if (target.dataset.separateRunToggle !== undefined) {
      session.runSeparate = target.checked;
      render();
      return;
    }
    if (target.dataset.separateDoNotUnload !== undefined) {
      session.selected = changedTriggerSelection(session.selected, "none", target.checked);
      render();
      return;
    }
    const trigger = target.dataset.separateTrigger;
    if (trigger === undefined) {
      return;
    }
    session.selected = changedTriggerSelection(session.selected, trigger, target.checked);
    render();
  });
  modelsElements.separateRuntimeDialog.addEventListener("click", event => {
    const target = event.target;
    if (!(target instanceof HTMLElement) || !session) {
      return;
    }
    if (target.dataset.separateAction === "cancel") {
      closeDialog();
      return;
    }
    if (target.dataset.separateAction === "save") {
      void submit(refreshInventory);
    }
  });
}

export async function openSeparateRuntimeDialog(nodeId: string, localId: string): Promise<void> {
  const response = await fetchSeparateRuntime(nodeId, localId);
  session = {
    nodeId: response.node_id || nodeId,
    localId: response.local_id || localId,
    runSeparate: response.settings.run_separate,
    selected: selectedTriggerKeys(response.settings),
    candidates: response.candidates
  };
  render();
  modelsElements.separateRuntimeDialog.showModal();
}

function closeDialog(): void {
  session = null;
  if (modelsElements.separateRuntimeDialog.open) {
    modelsElements.separateRuntimeDialog.close();
  }
}

async function submit(refreshInventory: () => Promise<void>): Promise<void> {
  if (!session) {
    return;
  }
  setStatus("Saving…", false);
  try {
    await saveSeparateRuntime({
      node_id: session.nodeId,
      local_id: session.localId,
      settings: {
        run_separate: session.runSeparate,
        triggers: triggersFromSelection(session.selected)
      }
    });
    closeDialog();
    await refreshInventory();
  } catch (error) {
    setStatus(errorMessage(error), true);
  }
}

function setStatus(message: string, error: boolean): void {
  showStatus(modelsElements.separateRuntimeDialogStatus, message, error);
}

function render(): void {
  if (!session) {
    return;
  }
  setHTML(modelsElements.separateRuntimeDialogBody, html`
    <h2>Separate runtime · ${session.localId}</h2>
    <p class="dialog-note">A separate config runs in its own backend process on this node, so another config's load, switch or unload never touches it.</p>
    <label class="routing-candidate">
      ${checkbox({"data-separate-run-toggle": true}, session.runSeparate)}
      <span class="routing-candidate-name">Run in its own process</span>
    </label>
    ${session.runSeparate ? triggerSection(session) : ""}
    ${dialogActions([
      button("Cancel", {"data-separate-action": "cancel"}),
      primaryButton("Save", {"data-separate-action": "save"})
    ])}
  `);
  setStatus("", false);
}

function triggerSection(current: DialogSession): SafeHTML {
  const groups = groupTriggersByKind(current.candidates);
  const doNotUnload = doNotUnloadSelected({run_separate: current.runSeparate, triggers: [...current.selected]});
  return html`
    <p class="dialog-note">Pick which loads on the shared runtime evict this one. A full pool still unloads the least-recently-used separate runtime regardless.</p>
    <label class="routing-candidate">
      ${checkbox({"data-separate-do-not-unload": true}, doNotUnload)}
      <span class="routing-candidate-name">Do not unload — no trigger evicts this runtime</span>
    </label>
    ${triggerGroup("Lanes", groups.lanes, current.selected)}
    ${triggerGroup("Backends", groups.families, current.selected)}
    ${triggerGroup("Configs", groups.configs, current.selected)}
  `;
}

function triggerGroup(title: string, triggers: string[], selected: Set<string>): SafeHTML {
  if (triggers.length === 0) {
    return emptyHTML;
  }
  const rows = html`${triggers.map(trigger => triggerOption(trigger, selected.has(trigger)))}`;
  return html`<div class="routing-node"><h3>${title}</h3>${rows}</div>`;
}

function triggerOption(trigger: string, checked: boolean): SafeHTML {
  return html`
    <label class="routing-candidate">
      ${checkbox({"data-separate-trigger": trigger}, checked)}
      <span class="routing-candidate-name">${triggerLabel(trigger)}</span>
    </label>
  `;
}


function triggerLabel(trigger: string): string {
  if (trigger in unloadPolicyLabels) {
    return unloadPolicyLabels[trigger as keyof typeof unloadPolicyLabels];
  }
  if (trigger.startsWith("config:")) {
    return trigger.slice("config:".length);
  }
  return trigger;
}
