import { confirmDestructive } from "./dialogs";
import { elements } from "./elements";
import { state } from "./state";

export function registerDirtyStateGuard(): void {
  window.addEventListener("beforeunload", event => {
    if (!hasDirtyWork()) {
      return;
    }
    event.preventDefault();
    event.returnValue = "";
  });
}

export function hasDirtyWork(): boolean {
  return simpleCookDirty() || constructorDirty();
}

export async function confirmDiscardDirtyWork(action: string): Promise<boolean> {
  if (!hasDirtyWork()) {
    return true;
  }
  return confirmDestructive("Discard unsaved work?", `${action} will discard changes that have not been applied.`, "Discard changes");
}

export function markSimpleCookClean(): void {
  state.simpleCook.cleanFields = clone(state.simpleCook.fields);
  state.simpleCook.cleanID = elements.cookIdInput.value.trim();
}

export function markConstructorClean(): void {
  state.constructor.cleanSnapshot = constructorSnapshot();
}

function simpleCookDirty(): boolean {
  return JSON.stringify(state.simpleCook.fields) !== JSON.stringify(state.simpleCook.cleanFields) ||
    elements.cookIdInput.value.trim() !== state.simpleCook.cleanID;
}

function constructorDirty(): boolean {
  return constructorSnapshot() !== state.constructor.cleanSnapshot;
}

function constructorSnapshot(): string {
  return JSON.stringify({
    lanes: state.constructor.lanes,
    targets: state.constructor.targetNodes,
    laneOptions: state.constructor.laneOptions,
    backendMode: state.constructor.backendMode,
    backendTouched: state.constructor.backendTouched,
    options: state.constructor.options,
    id: elements.advancedCookIdInput.value.trim()
  });
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}
