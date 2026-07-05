import { loadModelConfig, unloadModelConfig } from "./api";
import { elements } from "./elements";

export async function loadSelectedConfig(modelID: string, refreshInventory: () => Promise<void>): Promise<void> {
  const id = modelID.trim();
  if (!id) {
    return;
  }
  setModelActionStatus(`Loading ${id}...`, false);
  try {
    await loadModelConfig({model: id, unload_policy: elements.loadPolicySelect.value});
    setModelActionStatus(`Loaded ${id}`, false);
    await refreshInventory();
  } catch (error) {
    setModelActionStatus(error instanceof Error ? error.message : String(error), true);
  }
}

export async function unloadSelectedTarget(refreshInventory: () => Promise<void>): Promise<void> {
  const target = elements.unloadTargetSelect.value;
  setModelActionStatus(`Unloading ${target}...`, false);
  try {
    await unloadModelConfig({target});
    setModelActionStatus(`Unloaded ${target}`, false);
    await refreshInventory();
  } catch (error) {
    setModelActionStatus(error instanceof Error ? error.message : String(error), true);
  }
}

function setModelActionStatus(message: string, error: boolean): void {
  elements.modelsActionStatus.textContent = message;
  elements.modelsActionStatus.classList.toggle("error-text", error);
}
