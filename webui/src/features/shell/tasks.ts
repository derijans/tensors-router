import { runOperation } from "./operations";

export function runTask(task: () => Promise<void>, key = "general", group = "general", label = "Working…"): void {
  void runOperation({key, group, label, task}).catch(() => undefined);
}

