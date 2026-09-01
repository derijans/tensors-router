import { fetchSeparateRuntime, saveSeparateRuntime } from "./api";
import { unloadPolicyLabels } from "./constants";
import { elements } from "./elements";
import {
  changedTriggerSelection,
  doNotUnloadSelected,
  groupTriggersByKind,
  selectedTriggerKeys,
  triggersFromSelection
} from "./separate-runtime-data";
import type { SeparateRuntimeCandidates } from "./types";
import { escapeAttribute, escapeHTML } from "./utils";

interface DialogSession {
  nodeId: string;
  localId: string;
  runSeparate: boolean;
  selected: Set<string>;
  candidates: SeparateRuntimeCandidates;
}

let session: DialogSession | null = null;

export function registerSeparateRuntimeDialog(refreshInventory: () => Promise<void>): void {
  elements.separateRuntimeDialog.addEventListener("cancel", event => {
    event.preventDefault();
    closeDialog();
  });
  elements.separateRuntimeDialog.addEventListener("change", event => {
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
  elements.separateRuntimeDialog.addEventListener("click", event => {
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
  elements.separateRuntimeDialog.showModal();
}

function closeDialog(): void {
  session = null;
  if (elements.separateRuntimeDialog.open) {
    elements.separateRuntimeDialog.close();
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
    setStatus(error instanceof Error ? error.message : String(error), true);
  }
}

function setStatus(message: string, error: boolean): void {
  elements.separateRuntimeDialogStatus.textContent = message;
  elements.separateRuntimeDialogStatus.classList.toggle("error-text", error);
}

function render(): void {
  if (!session) {
    return;
  }
  elements.separateRuntimeDialogBody.innerHTML = `
    <h2>Separate runtime · ${escapeHTML(session.localId)}</h2>
    <p class="dialog-note">A separate config runs in its own backend process on this node, so another config's load, switch or unload never touches it.</p>
    <label class="routing-candidate">
      <input type="checkbox" data-separate-run-toggle${session.runSeparate ? " checked" : ""}>
      <span class="routing-candidate-name">Run in its own process</span>
    </label>
    ${session.runSeparate ? triggerSection(session) : ""}
    <div class="dialog-actions">
      <button type="button" data-separate-action="cancel">Cancel</button>
      <button type="button" data-separate-action="save">Save</button>
    </div>
  `;
  setStatus("", false);
}

function triggerSection(current: DialogSession): string {
  const groups = groupTriggersByKind(current.candidates);
  const doNotUnload = doNotUnloadSelected({run_separate: current.runSeparate, triggers: [...current.selected]});
  return `
    <p class="dialog-note">Pick which loads on the shared runtime evict this one. A full pool still unloads the least-recently-used separate runtime regardless.</p>
    <label class="routing-candidate">
      <input type="checkbox" data-separate-do-not-unload${doNotUnload ? " checked" : ""}>
      <span class="routing-candidate-name">Do not unload — no trigger evicts this runtime</span>
    </label>
    ${triggerGroup("Lanes", groups.lanes, current.selected)}
    ${triggerGroup("Backends", groups.families, current.selected)}
    ${triggerGroup("Configs", groups.configs, current.selected)}
  `;
}

function triggerGroup(title: string, triggers: string[], selected: Set<string>): string {
  if (triggers.length === 0) {
    return "";
  }
  const rows = triggers.map(trigger => `
    <label class="routing-candidate">
      <input type="checkbox" data-separate-trigger="${escapeAttribute(trigger)}"${selected.has(trigger) ? " checked" : ""}>
      <span class="routing-candidate-name">${escapeHTML(triggerLabel(trigger))}</span>
    </label>
  `).join("");
  return `<div class="routing-node"><h3>${escapeHTML(title)}</h3>${rows}</div>`;
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
