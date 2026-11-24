import { elementTarget } from "../../lib/dom";
import { runTask } from "../shell/tasks";
import { closeSecurityDialog, copyDisplayedSecret, downloadDialogPart } from "./security-dialog-shell";
import { securityElements } from "./security-elements";
import { submitSecurityForm } from "./security-form-submit";

export const securityGroup = "security";

export function bindSecurityDialog(): void {
  securityElements.dialog.addEventListener("cancel", event => {
    event.preventDefault();
    closeSecurityDialog();
  });
  securityElements.dialog.addEventListener("submit", event => {
    const form = elementTarget(event)?.closest("form");
    if (form) {
      event.preventDefault();
      runTask(() => submitSecurityForm(form), "security-dialog", securityGroup, "Saving…");
    }
  });
  securityElements.dialog.addEventListener("click", event => {
    const target = elementTarget(event)?.closest<HTMLElement>("button");
    if (!target) {
      return;
    }
    if (target.dataset.securityDialogClose !== undefined) {
      closeSecurityDialog();
    } else if (target.dataset.securityCopySecret !== undefined) {
      runTask(copyDisplayedSecret, "security-copy", securityGroup, "Copying…");
    } else if (target.dataset.securityDownload) {
      downloadDialogPart(target.dataset.securityDownload);
    }
  });
}
