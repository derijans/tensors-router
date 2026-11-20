import { forceKillRouter, launchRouter, login, logout, restartRouter, shutdownRouter } from "../../api/session-api";
import { setCsrfToken } from "../../api/client";
import { refreshAll } from "./refresh";
import { clearConversionWarnings } from "../cook/conversions";
import { confirmDestructive } from "./dialogs";
import { confirmDiscardDirtyWork, markConstructorClean, markSimpleCookClean } from "./dirty-state";
import { stopNodeStatePolling } from "../nodes/nodes-polling";
import { stopOverviewPolling } from "../overview/overview";
import { renderRouterStatus, showApp, showLogin } from "./render-dashboard";
import { activateTab } from "./navigation";
import { state } from "../../state/app-state";
import { errorMessage } from "../../lib/errors";
import { runTask } from "./tasks";
import type { RouterProcessStatus } from "../../types";
import { loadWebUIs } from "../webuis/webuis";
import { shellElements } from "./shell-elements";

export function bindSession(): void {
  shellElements.loginForm.addEventListener("submit", event => {
    event.preventDefault();
    void submitLogin();
  });
  shellElements.logoutButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDiscardDirtyWork("Logging out")) {
      await handleLogout();
    }
  }, "logout", "session", "Logging out…"));
  shellElements.refreshButton.addEventListener("click", () => runTask(refreshAll, "refresh-all", "refresh", "Refreshing data…"));
}

export function bindRouterControls(): void {
  shellElements.launchButton.addEventListener("click", () => runTask(() => applyRouterAction(launchRouter), "router-launch", "router", "Launching router…"));
  shellElements.restartButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDestructive("Restart router?", "Active requests may be interrupted if they cannot finish during the drain period.", "Restart")) {
      await applyRouterAction(restartRouter);
    }
  }, "router-restart", "router", "Restarting router…"));
  shellElements.shutdownButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDestructive("Stop router?", "The router will stop accepting new work and drain active transfers.", "Stop")) {
      await applyRouterAction(shutdownRouter);
    }
  }, "router-shutdown", "router", "Stopping router…"));
  shellElements.forceKillButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDestructive("Force-kill router?", "Active requests will be terminated immediately and may fail.", "Force kill")) {
      await applyRouterAction(forceKillRouter);
    }
  }, "router-force-kill", "router", "Force-killing router…"));
}

async function submitLogin(): Promise<void> {
  shellElements.loginError.textContent = "";
  try {
    const session = await login(shellElements.tokenInput.value);
    setCsrfToken(session.csrf);
    showApp();
    await refreshAll();
    activateTab(state.activeTab);
  } catch (error) {
    shellElements.loginError.textContent = errorMessage(error);
  }
}

async function handleLogout(): Promise<void> {
  await logout();
  stopNodeStatePolling();
  stopOverviewPolling();
  setCsrfToken("");
  markSimpleCookClean();
  markConstructorClean();
  clearConversionWarnings();
  showLogin();
}

async function applyRouterAction(action: () => Promise<RouterProcessStatus>): Promise<void> {
  if (shellElements.routerMenu.matches(":popover-open")) {
    shellElements.routerMenu.hidePopover();
  }
  state.router = await action();
  renderRouterStatus();
  await loadWebUIs();
}
