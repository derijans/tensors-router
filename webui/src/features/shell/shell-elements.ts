import { getRequiredElement } from "../../lib/dom";

export const shellElements = {
  loginView: getRequiredElement("loginView", HTMLElement),
  appView: getRequiredElement("appView", HTMLElement),
  loginForm: getRequiredElement("loginForm", HTMLFormElement),
  tokenInput: getRequiredElement("tokenInput", HTMLInputElement),
  loginError: getRequiredElement("loginError", HTMLElement),
  operationStatus: getRequiredElement("operationStatus", HTMLElement),
  logoutButton: getRequiredElement("logoutButton", HTMLButtonElement),
  refreshButton: getRequiredElement("refreshButton", HTMLButtonElement),
  launchButton: getRequiredElement("launchButton", HTMLButtonElement),
  restartButton: getRequiredElement("restartButton", HTMLButtonElement),
  shutdownButton: getRequiredElement("shutdownButton", HTMLButtonElement),
  forceKillButton: getRequiredElement("forceKillButton", HTMLButtonElement),
  routerSummary: getRequiredElement("routerSummary", HTMLElement),
  routerStatus: getRequiredElement("routerStatus", HTMLElement),
  routerMenu: getRequiredElement("routerMenu", HTMLElement),
  shellSection: getRequiredElement("shellSection", HTMLElement),
  recipeCount: getRequiredElement("recipeCount", HTMLElement),
  safetyDialog: getRequiredElement("safetyDialog", HTMLDialogElement),
  safetyDialogBody: getRequiredElement("safetyDialogBody", HTMLElement)
};
