import { applyCook, errorBody, previewCook } from "./api";
import { elements } from "./elements";
import { advancedCookRequest, localValidation } from "./constructor-data";
import { renderConstructor } from "./constructor";
import { clearConstructorConversions, confirmPendingConversions } from "./conversions";
import { cookResultHTML } from "./cook-result";
import { confirmDestructive } from "./dialogs";
import { markConstructorClean } from "./dirty-state";
import type { CookRequest, CookResponse, RefreshInventory } from "./types";

export async function previewAdvancedCook(): Promise<void> {
  if (!await confirmPendingConversions("constructor")) {
    return;
  }
  await submitCook(previewCook, advancedCookRequest());
}

export async function applyAdvancedCook(refreshInventory: RefreshInventory): Promise<void> {
  if (!await confirmPendingConversions("constructor")) {
    return;
  }
  const errors = localValidation().filter(item => item.severity === "error");
  if (errors.length > 0) {
    renderConstructor();
    elements.cookOutput.innerHTML = cookResultHTML({error: "Validation failed", validation: errors});
    return;
  }
  const request = advancedCookRequest();
  if (request.overwrite && !await confirmDestructive("Overwrite configurations?", `Applying ${request.id || "this cook plan"} may replace existing configurations.`, "Overwrite")) {
    return;
  }
  await submitCook(applyCook, request);
  await refreshInventory();
  markConstructorClean();
  clearConstructorConversions();
}

async function submitCook(submit: (request: CookRequest) => Promise<unknown>, request: CookRequest): Promise<void> {
  try {
    const result = await submit(request);
    elements.cookOutput.innerHTML = cookResultHTML(result as CookResponse);
    renderConstructor();
  } catch (error) {
    elements.cookOutput.innerHTML = cookResultHTML(errorBody(error));
    renderConstructor();
    throw error;
  }
}
