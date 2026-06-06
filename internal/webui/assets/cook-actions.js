import { api } from "./api.js";
import { elements } from "./elements.js";
import { advancedCookRequest, localValidation, renderConstructor } from "./constructor.js";

export async function previewAdvancedCook() {
  await submitCook("/api/cook/preview", advancedCookRequest());
}

export async function applyAdvancedCook(refreshInventory) {
  const errors = localValidation().filter(item => item.severity === "error");
  if (errors.length > 0) {
    renderConstructor();
    elements.cookOutput.textContent = JSON.stringify({validation: errors}, null, 2);
    return;
  }
  await submitCook("/api/cook/apply", advancedCookRequest());
  await refreshInventory();
}

async function submitCook(path, request) {
  try {
    const result = await api(path, {
      method: "POST",
      body: JSON.stringify(request)
    });
    elements.cookOutput.textContent = JSON.stringify(result, null, 2);
    renderConstructor();
  } catch (error) {
    const body = error.data?.validation ? {error: error.message, validation: error.data.validation} : {error: error.message};
    elements.cookOutput.textContent = JSON.stringify(body, null, 2);
    renderConstructor();
  }
}
