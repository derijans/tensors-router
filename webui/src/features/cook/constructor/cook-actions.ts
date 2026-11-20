import { setHTML } from "../../../lib/safe-html";
import { applyCook, previewCook } from "../../../api/cook-api";
import { errorBody } from "../../../api/client";
import { currentAdvancedCookRequest, currentValidationIssues } from "./constructor-current-request";
import { renderConstructor } from "./constructor-render";
import { clearConstructorConversions, confirmPendingConversions } from "../conversions";
import { cookResultHTML } from "../cook-result-view";
import { confirmDestructive } from "../../shell/dialogs";
import { markConstructorClean } from "../../shell/dirty-state";
import type { CookRequest, CookResponse, RefreshInventory } from "../../../types";
import { cookElements } from "../cook-elements";

export async function previewAdvancedCook(): Promise<void> {
  if (!await confirmPendingConversions("constructor")) {
    return;
  }
  await submitCook(previewCook, currentAdvancedCookRequest());
}

export async function applyAdvancedCook(refreshInventory: RefreshInventory): Promise<void> {
  if (!await confirmPendingConversions("constructor")) {
    return;
  }
  const errors = currentValidationIssues().filter(item => item.severity === "error");
  if (errors.length > 0) {
    renderConstructor();
    setHTML(cookElements.cookOutput, cookResultHTML({error: "Validation failed", validation: errors}));
    return;
  }
  const request = currentAdvancedCookRequest();
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
    setHTML(cookElements.cookOutput, cookResultHTML(result as CookResponse));
    renderConstructor();
  } catch (error) {
    setHTML(cookElements.cookOutput, cookResultHTML(errorBody(error)));
    renderConstructor();
    throw error;
  }
}
