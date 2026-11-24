import { errorMessage } from "../../lib/errors";
import { securityDialogTarget, showSecurityDialogError } from "./security-dialog-shell";
import { type SecurityFormName, isSecurityFormName } from "./security-form-frame-view";
import type { SecurityFormSubmitter } from "./security-form-submitter";
import { localFormSubmitters } from "./security-local-submitters";
import { nodeFormSubmitters } from "./security-node-submitters";
import { targetFormSubmitters } from "./security-target-submitters";

const formSubmitters: Record<SecurityFormName, SecurityFormSubmitter> = {
  ...targetFormSubmitters,
  ...localFormSubmitters,
  ...nodeFormSubmitters
};

export async function submitSecurityForm(form: HTMLFormElement): Promise<void> {
  const name = form.dataset.securityForm;
  if (!isSecurityFormName(name)) {
    return;
  }
  try {
    await formSubmitters[name](new FormData(form), securityDialogTarget());
  } catch (error) {
    showSecurityDialogError(errorMessage(error));
  }
}
