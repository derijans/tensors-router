import { fetchRoutingLinks, getInventory, getRouterStatus } from "./api";
import { loadAnalytics } from "./analytics";
import { loadDownloads } from "./downloads";
import { loadLending } from "./lending-tab";
import { loadLoadCaptures } from "./load-captures";
import { loadLoadErrors } from "./load-errors";
import { loadOverview } from "./overview/overview";
import { renderInventory, renderRouterStatus } from "./render-dashboard";
import { state } from "./state";
import { loadWebUIs } from "./webuis";

export async function refreshAll(): Promise<void> {
  await refreshRouterStatus();
  await refreshInventory(state.activeTab === "models");
  await loadWebUIs();
  await loadAnalytics();
  await loadDownloads();
  await loadLoadCaptures();
  await loadLoadErrors();
  await loadLending();
  await loadOverview();
}

export async function refreshRouterStatus(): Promise<void> {
  state.router = await getRouterStatus();
  renderRouterStatus();
}

export async function refreshInventory(includeFiles = state.activeTab === "models"): Promise<void> {
  state.inventory = await getInventory(includeFiles);
  const [imageLinks, textLinks] = await Promise.all([
    fetchRoutingLinks("image").catch(() => null),
    fetchRoutingLinks("text").catch(() => null)
  ]);
  state.routingLinks = {image: imageLinks, text: textLinks};
  renderInventory();
}
