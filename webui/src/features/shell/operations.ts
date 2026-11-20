import { errorMessage } from "../../lib/errors";
import { html, setHTML } from "../../lib/safe-html";
import { state } from "../../state/app-state";
import { errorText } from "../../ui/text";
import { shellElements } from "./shell-elements";

const modelMutationGroups = new Set(["router", "cook", "cook-selection", "webui", "benchmark"]);
const unavailableAttribute = "data-unavailable";

type OperationControl = HTMLButtonElement | HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;

export interface OperationRequest<T> {
  key: string;
  group: string;
  label: string;
  task: () => Promise<T>;
}

export async function runOperation<T>(request: OperationRequest<T>): Promise<T | undefined> {
  if (groupPending(request.group)) {
    return undefined;
  }
  const execute = async (): Promise<T> => {
    state.operations[request.key] = {
      key: request.key,
      group: request.group,
      label: request.label,
      pending: true,
      error: ""
    };
    renderOperations();
    try {
      const value = await request.task();
      delete state.operations[request.key];
      renderOperations();
      return value;
    } catch (error) {
      state.operations[request.key] = {
        key: request.key,
        group: request.group,
        label: request.label,
        pending: false,
        error: errorMessage(error),
        retry: execute
      };
      renderOperations();
      throw error;
    }
  };
  return execute();
}

export function registerOperationRetry(): void {
  shellElements.operationStatus.addEventListener("click", event => {
    const target = event.target;
    if (!(target instanceof HTMLElement)) {
      return;
    }
    const key = target.dataset.retryOperation;
    const retry = key ? state.operations[key]?.retry : undefined;
    if (retry) {
      void retry().catch(() => undefined);
    }
  });
}

export function setControlUnavailable(control: OperationControl, unavailable: boolean): void {
  control.toggleAttribute(unavailableAttribute, unavailable);
  const group = control.dataset.operationGroup;
  control.disabled = unavailable || (group !== undefined && groupPending(group));
}

export function groupPending(group: string): boolean {
  return Object.values(state.operations).some(operation => operation.pending && operationGroupsConflict(group, operation.group));
}

function renderOperations(): void {
  const operations = Object.values(state.operations);
  const pendingGroups = operations.filter(operation => operation.pending).map(operation => operation.group);
  document.querySelectorAll<HTMLElement>("[data-operation-group]").forEach(element => {
    if (element instanceof HTMLButtonElement || element instanceof HTMLInputElement || element instanceof HTMLSelectElement) {
      element.disabled = element.hasAttribute(unavailableAttribute) || pendingGroups.some(group => operationGroupsConflict(element.dataset.operationGroup || "", group));
    }
  });
  const pending = operations.find(operation => operation.pending);
  if (pending) {
    setHTML(shellElements.operationStatus, html`<span>${pending.label}</span>`);
    return;
  }
  const failed = operations.find(operation => operation.error);
  if (failed) {
    setHTML(shellElements.operationStatus, html`
      ${errorText(failed.error, "span")}
      <button type="button" data-retry-operation="${failed.key}">Retry</button>
    `);
    return;
  }
  shellElements.operationStatus.textContent = "";
}

function operationGroupsConflict(left: string, right: string): boolean {
  if (left === right || left === "session" || right === "session" || left === "refresh" || right === "refresh") {
    return true;
  }
  return modelMutationGroups.has(left) && modelMutationGroups.has(right);
}

