import { state } from "./state";
import { elements } from "./elements";
import { renderAnalytics } from "./analytics";
import { renderConstructor } from "./constructor";
import { renderBenchmarks } from "./benchmarks";
import { renderSimpleCook } from "./simple-cook";
import { renderModelInventory } from "./model-inventory";
import { renderNodesPanel } from "./nodes-state";
import {
  escapeAttribute,
  escapeHTML,
  statusItem
} from "./utils";

export function showLogin(): void {
  elements.loginView.classList.remove("hidden");
  elements.appView.classList.add("hidden");
}

export function showApp(): void {
  elements.loginView.classList.add("hidden");
  elements.appView.classList.remove("hidden");
}

export function renderInventory(): void {
  renderNodesPanel();
  renderTables();
  renderBenchmarks();
  renderAnalytics();
  renderSimpleCook();
  renderConstructor();
  renderRecipes();
}

export function renderRouterStatus(): void {
  const router = state.router;
  elements.routerSummary.textContent = `${router?.url || ""} ${router?.running ? "running" : "stopped"}`;
  elements.launchButton.disabled = !router?.managed || Boolean(router?.running);
  elements.restartButton.disabled = !router?.managed;
  elements.shutdownButton.disabled = !router?.can_shutdown;
  elements.forceKillButton.disabled = !router?.can_force_kill;
  elements.routerStatus.innerHTML = [
    statusItem("Managed", router?.managed ? "yes" : "no"),
    statusItem("Running", router?.running ? "yes" : "no"),
    statusItem("URL", router?.url || "unknown"),
    statusItem("PID", router?.pid ? String(router.pid) : "none"),
    statusItem("Can shutdown", router?.can_shutdown ? "yes" : "no"),
    statusItem("Can force kill", router?.can_force_kill ? "yes" : "no"),
    statusItem("Last error", router?.error || "none")
  ].join("");
}

export function renderTables(): void {
  renderModelInventory();
}

export function renderRecipes(): void {
  const recipes = state.inventory?.recipes ?? [];
  elements.recipeCount.textContent = `${recipes.length} recipes`;
  elements.recipesList.innerHTML = recipes.map(recipe => `
    <article class="recipe-item">
      <div>
        <strong>${escapeHTML(recipe.public_id || recipe.id)}</strong>
        <div class="muted">${escapeHTML(recipe.public_image_id || "")}</div>
      </div>
      <button type="button" data-delete-recipe="${escapeAttribute(recipe.id)}">Delete</button>
    </article>
  `).join("");
}
