import { elements } from "./elements";
import { state } from "./state";
import { escapeAttribute, escapeHTML } from "./utils";

const modelMutationGroups = new Set(["router", "cook", "cook-selection", "webui", "benchmark"]);

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
  elements.operationStatus.addEventListener("click", event => {
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

export function groupPending(group: string): boolean {
  return Object.values(state.operations).some(operation => operation.pending && operationGroupsConflict(group, operation.group));
}

function renderOperations(): void {
  const operations = Object.values(state.operations);
  const pendingGroups = operations.filter(operation => operation.pending).map(operation => operation.group);
  document.querySelectorAll<HTMLElement>("[data-operation-group]").forEach(element => {
    if (element instanceof HTMLButtonElement || element instanceof HTMLInputElement || element instanceof HTMLSelectElement) {
      element.disabled = pendingGroups.some(group => operationGroupsConflict(element.dataset.operationGroup || "", group));
    }
  });
  const pending = operations.find(operation => operation.pending);
  if (pending) {
    elements.operationStatus.innerHTML = `<span>${escapeHTML(pending.label)}</span>`;
    return;
  }
  const failed = operations.find(operation => operation.error);
  if (failed) {
    elements.operationStatus.innerHTML = `
      <span class="error-text">${escapeHTML(failed.error)}</span>
      <button type="button" data-retry-operation="${escapeAttribute(failed.key)}">Retry</button>
    `;
    return;
  }
  elements.operationStatus.textContent = "";
}

function operationGroupsConflict(left: string, right: string): boolean {
  if (left === right || left === "session" || right === "session" || left === "refresh" || right === "refresh") {
    return true;
  }
  return modelMutationGroups.has(left) && modelMutationGroups.has(right);
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
