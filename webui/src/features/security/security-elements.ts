import { getRequiredElement } from "../../lib/dom";

export const securityElements = {
  listenerNotice: getRequiredElement("securityListenerNotice", HTMLElement),
  profileNotice: getRequiredElement("securityProfileNotice", HTMLElement),
  profileMessage: getRequiredElement("securityProfileMessage", HTMLElement),
  secureProfileButton: getRequiredElement("securitySecureProfileButton", HTMLButtonElement),
  status: getRequiredElement("securityStatus", HTMLElement),
  tokenRows: getRequiredElement("securityTokenRows", HTMLTableSectionElement),
  certificateRows: getRequiredElement("securityCertificateRows", HTMLTableSectionElement),
  createTokenButton: getRequiredElement("securityCreateTokenButton", HTMLButtonElement),
  generateIdentityButton: getRequiredElement("securityGenerateIdentityButton", HTMLButtonElement),
  importCertificateButton: getRequiredElement("securityImportCertificateButton", HTMLButtonElement),
  createAuthorityButton: getRequiredElement("securityCreateAuthorityButton", HTMLButtonElement),
  dialog: getRequiredElement("securityDialog", HTMLDialogElement),
  dialogBody: getRequiredElement("securityDialogBody", HTMLElement),
  dialogStatus: getRequiredElement("securityDialogStatus", HTMLElement)
};
