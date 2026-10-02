import { parsedModelColumnChoices, type ModelColumnChoices } from "./model-columns";

const columnChoicesStorageKey = "tensors-router.models.columns";

export function loadModelColumnChoices(): ModelColumnChoices {
  try {
    return parsedModelColumnChoices(JSON.parse(window.localStorage.getItem(columnChoicesStorageKey) || "{}"));
  } catch {
    return {};
  }
}

export function saveModelColumnChoices(choices: ModelColumnChoices): void {
  try {
    window.localStorage.setItem(columnChoicesStorageKey, JSON.stringify(choices));
  } catch {
    return;
  }
}
