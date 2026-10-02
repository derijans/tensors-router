import { forceKillRouter, launchRouter, login, logout, restartRouter, shutdownRouter } from "../api";
import { refreshAll } from "../app-data";
import { clearConversionWarnings } from "../conversions";
import { confirmDestructive } from "../dialogs";
import { confirmDiscardDirtyWork, markConstructorClean, markSimpleCookClean } from "../dirty-state";
import { elements } from "../elements";
import { stopNodeStatePolling } from "../nodes-state";
import { stopOverviewPolling } from "../overview/overview";
import { renderRouterStatus, showApp, showLogin } from "../render-dashboard";
import { activateTab } from "../shell/navigation";
import { state } from "../state";
import { errorMessage, runTask } from "../tasks";
import type { RouterProcessStatus } from "../types";
import { loadWebUIs } from "../webuis";

export function bindSession(): void {
  elements.loginForm.addEventListener("submit", event => {
    event.preventDefault();
    void submitLogin();
  });
  elements.logoutButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDiscardDirtyWork("Logging out")) {
      await handleLogout();
    }
  }, "logout", "session", "Logging out…"));
  elements.refreshButton.addEventListener("click", () => runTask(refreshAll, "refresh-all", "refresh", "Refreshing data…"));
}

export function bindRouterControls(): void {
  elements.launchButton.addEventListener("click", () => runTask(() => applyRouterAction(launchRouter), "router-launch", "router", "Launching router…"));
  elements.restartButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDestructive("Restart router?", "Active requests may be interrupted if they cannot finish during the drain period.", "Restart")) {
      await applyRouterAction(restartRouter);
    }
  }, "router-restart", "router", "Restarting router…"));
  elements.shutdownButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDestructive("Stop router?", "The router will stop accepting new work and drain active transfers.", "Stop")) {
      await applyRouterAction(shutdownRouter);
    }
  }, "router-shutdown", "router", "Stopping router…"));
  elements.forceKillButton.addEventListener("click", () => runTask(async () => {
    if (await confirmDestructive("Force-kill router?", "Active requests will be terminated immediately and may fail.", "Force kill")) {
      await applyRouterAction(forceKillRouter);
    }
  }, "router-force-kill", "router", "Force-killing router…"));
}

async function submitLogin(): Promise<void> {
  elements.loginError.textContent = "";
  try {
    const session = await login(elements.tokenInput.value);
    state.csrf = session.csrf;
    showApp();
    await refreshAll();
    activateTab(state.activeTab);
  } catch (error) {
    elements.loginError.textContent = errorMessage(error);
  }
}

async function handleLogout(): Promise<void> {
  await logout();
  stopNodeStatePolling();
  stopOverviewPolling();
  state.csrf = "";
  markSimpleCookClean();
  markConstructorClean();
  clearConversionWarnings();
  showLogin();
}

async function applyRouterAction(action: () => Promise<RouterProcessStatus>): Promise<void> {
  if (elements.routerMenu.matches(":popover-open")) {
    elements.routerMenu.hidePopover();
  }
  state.router = await action();
  renderRouterStatus();
  await loadWebUIs();
}
