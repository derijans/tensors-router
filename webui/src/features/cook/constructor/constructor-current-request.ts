import { state } from "../../../state/app-state";
import type { ConstructorContext, CookRequest, ValidationIssue } from "../../../types";
import { cookElements } from "../cook-elements";
import { advancedCookRequest, type CookTarget, localValidationIssues } from "./constructor-request-data";

export function currentConstructorContext(): ConstructorContext {
  return {draft: state.constructor, inventory: state.inventory};
}

export function currentAdvancedCookRequest(): CookRequest {
  return advancedCookRequest(currentConstructorContext(), currentCookTarget());
}

export function currentValidationIssues(): ValidationIssue[] {
  return localValidationIssues(currentConstructorContext(), currentAdvancedCookRequest());
}

function currentCookTarget(): CookTarget {
  return {
    id: cookElements.advancedCookIdInput.value,
    overwrite: cookElements.overwriteInput.checked
  };
}
