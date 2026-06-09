import { applyCook, errorBody, previewCook } from "./api";
import { elements } from "./elements";
import { advancedCookRequest, localValidation } from "./constructor-data";
import { renderConstructor } from "./constructor";
import type { CookRequest, RefreshInventory } from "./types";

export async function previewAdvancedCook(): Promise<void> {
  await submitCook(previewCook, advancedCookRequest());
}

export async function applyAdvancedCook(refreshInventory: RefreshInventory): Promise<void> {
  const errors = localValidation().filter(item => item.severity === "error");
  if (errors.length > 0) {
    renderConstructor();
    elements.cookOutput.textContent = JSON.stringify({validation: errors}, null, 2);
    return;
  }
  await submitCook(applyCook, advancedCookRequest());
  await refreshInventory();
}

async function submitCook(submit: (request: CookRequest) => Promise<unknown>, request: CookRequest): Promise<void> {
  try {
    const result = await submit(request);
    elements.cookOutput.textContent = JSON.stringify(result, null, 2);
    renderConstructor();
  } catch (error) {
    elements.cookOutput.textContent = JSON.stringify(errorBody(error), null, 2);
    renderConstructor();
  }
}
