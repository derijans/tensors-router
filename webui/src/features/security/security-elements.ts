import { getRequiredElement } from "../../lib/dom";

export const securityElements = {
  nodePicker: getRequiredElement("securityNodePicker", HTMLElement),
  nodeSelect: getRequiredElement("securityNodeSelect", HTMLSelectElement),
  listenerNotices: getRequiredElement("securityListenerNotices", HTMLElement),
  apiCertificateNotice: getRequiredElement("securityApiCertificateNotice", HTMLElement),
  profileNotice: getRequiredElement("securityProfileNotice", HTMLElement),
  profileMessage: getRequiredElement("securityProfileMessage", HTMLElement),
  secureProfileButton: getRequiredElement("securitySecureProfileButton", HTMLButtonElement),
  status: getRequiredElement("securityStatus", HTMLElement),
  tokenRows: getRequiredElement("securityTokenRows", HTMLTableSectionElement),
  certificateRows: getRequiredElement("securityCertificateRows", HTMLTableSectionElement),
  pendingKeyRows: getRequiredElement("securityPendingKeyRows", HTMLTableSectionElement),
  createTokenButton: getRequiredElement("securityCreateTokenButton", HTMLButtonElement),
  pushClusterTokenButton: getRequiredElement("securityPushClusterTokenButton", HTMLButtonElement),
  generateIdentityButton: getRequiredElement("securityGenerateIdentityButton", HTMLButtonElement),
  importCertificateButton: getRequiredElement("securityImportCertificateButton", HTMLButtonElement),
  createAuthorityButton: getRequiredElement("securityCreateAuthorityButton", HTMLButtonElement),
  issueFromMasterButton: getRequiredElement("securityIssueFromMasterButton", HTMLButtonElement),
  trustMasterButton: getRequiredElement("securityTrustMasterButton", HTMLButtonElement),
  createRequestButton: getRequiredElement("securityCreateRequestButton", HTMLButtonElement),
  installSignedButton: getRequiredElement("securityInstallSignedButton", HTMLButtonElement),
  dialog: getRequiredElement("securityDialog", HTMLDialogElement),
  dialogBody: getRequiredElement("securityDialogBody", HTMLElement),
  dialogStatus: getRequiredElement("securityDialogStatus", HTMLElement)
};
