import { runOperation } from "./operations";

export function runTask(task: () => Promise<void>, key = "general", group = "general", label = "Working…"): void {
  void runOperation({key, group, label, task}).catch(() => undefined);
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
