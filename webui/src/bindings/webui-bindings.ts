import { elementTarget } from "../dom";
import { elements } from "../elements";
import { runTask } from "../tasks";
import {
  closeWebUIDialog,
  loadSelectedWebUIModel,
  openSelectedWebUI,
  setWebUIEnabled,
  showSelectedWebUIDialog,
  updateWebUIFilter
} from "../webuis";

export function bindWebUIs(): void {
  elements.webuiFilterInput.addEventListener("input", () => updateWebUIFilter(elements.webuiFilterInput.value));
  elements.webuiGrid.addEventListener("click", event => {
    const target = elementTarget(event);
    const openID = target?.closest<HTMLElement>("[data-webui-open]")?.dataset.webuiOpen;
    if (openID) {
      openSelectedWebUI(openID);
      return;
    }
    const detailsID = target?.closest<HTMLElement>("[data-webui-details]")?.dataset.webuiDetails;
    if (detailsID) {
      showSelectedWebUIDialog(detailsID);
    }
  });
  elements.webuiGrid.addEventListener("change", event => {
    const target = elementTarget(event);
    const toggleID = target?.dataset.webuiToggle;
    if (toggleID && target instanceof HTMLInputElement) {
      runTask(() => setWebUIEnabled(toggleID, target.checked), `webui-toggle-${toggleID}`, "webui", "Updating backend UI…");
    }
  });
  elements.webuiDialog.addEventListener("cancel", event => {
    event.preventDefault();
    closeWebUIDialog();
  });
  elements.webuiDialog.addEventListener("click", event => {
    const target = elementTarget(event);
    if (target?.closest("[data-webui-dialog-close]")) {
      closeWebUIDialog();
      return;
    }
    const enableID = target?.dataset.webuiEnable;
    if (enableID) {
      runTask(() => setWebUIEnabled(enableID, true), `webui-enable-${enableID}`, "webui", "Enabling backend UI…");
      return;
    }
    const loadID = target?.dataset.webuiLoad;
    if (loadID) {
      runTask(() => loadSelectedWebUIModel(loadID, target.dataset.webuiLoadModel || "", target.dataset.webuiLoadImage || ""), `webui-load-${loadID}`, "webui", "Loading backend UI model…");
    }
  });
}
