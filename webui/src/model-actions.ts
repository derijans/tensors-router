import { loadModelConfig } from "./api";
import { elements } from "./elements";

export async function loadSelectedConfig(modelID: string, refreshInventory: () => Promise<void>): Promise<void> {
  const id = modelID.trim();
  if (!id) {
    return;
  }
  setModelActionStatus(`Loading ${id}...`, false);
  try {
    await loadModelConfig({model: id});
    setModelActionStatus(`Loaded ${id}`, false);
    await refreshInventory();
  } catch (error) {
    setModelActionStatus(error instanceof Error ? error.message : String(error), true);
  }
}

function setModelActionStatus(message: string, error: boolean): void {
  elements.modelsActionStatus.textContent = message;
  elements.modelsActionStatus.classList.toggle("error-text", error);
}
