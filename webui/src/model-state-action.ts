import type { ModelStateRequest } from "./types";

export async function persistModelEnabled(
  request: ModelStateRequest,
  update: (request: ModelStateRequest) => Promise<unknown>,
  refresh: () => Promise<void>,
  rollback: () => void
): Promise<void> {
  try {
    await update(request);
    await refresh();
  } catch (error) {
    rollback();
    throw error;
  }
}
