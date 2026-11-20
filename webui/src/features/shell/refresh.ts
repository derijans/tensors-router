import { fetchRoutingLinks, getInventory } from "../../api/models-api";
import { getRouterStatus } from "../../api/session-api";
import { loadMemoryFit } from "../models/memory-fit/memory-fit-state";
import { loadAnalytics } from "../insights/analytics/analytics";
import { loadDownloads } from "../downloads/downloads";
import { loadLending } from "../lending/lending-tab";
import { loadLoadCaptures } from "../insights/load-captures/load-captures";
import { loadLoadErrors } from "../insights/load-errors/load-errors";
import { loadOverview } from "../overview/overview";
import { renderInventory, renderRouterStatus } from "./render-dashboard";
import { state } from "../../state/app-state";
import { loadWebUIs } from "../webuis/webuis";

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
  const [, imageLinks, textLinks] = await Promise.all([
    loadMemoryFit(state.inventory.nodes),
    fetchRoutingLinks("image").catch(() => null),
    fetchRoutingLinks("text").catch(() => null)
  ]);
  state.routingLinks = {image: imageLinks, text: textLinks};
  renderInventory();
}
