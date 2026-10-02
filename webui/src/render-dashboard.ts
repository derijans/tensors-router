import { SafeHTML, html, setHTML } from "./safe-html";
import { state } from "./state";
import { elements } from "./elements";
import { renderAnalytics } from "./analytics";
import { renderConstructor } from "./constructor";
import { renderBenchmarks } from "./benchmarks";
import { renderSimpleCook } from "./simple-cook";
import { renderModelInventory } from "./model-inventory";
import { renderNodesPanel } from "./nodes-state";
import { renderOverview } from "./overview/overview";
import { badge, fact, laneAccent } from "./markup-primitives";
import type { Recipe, RecipeComponent, RouterProcessStatus, Tone } from "./types";

const recipeLanes = ["text", "image", "embeddings", "voice", "music"] as const;

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
  renderOverview();
}

export function renderRouterStatus(): void {
  const router = state.router;
  elements.routerSummary.textContent = routerSummaryText(router);
  elements.routerSummary.className = `router-chip tone-${routerTone(router)}`;
  elements.launchButton.disabled = !router?.managed || Boolean(router?.running);
  elements.restartButton.disabled = !router?.managed;
  elements.shutdownButton.disabled = !router?.can_shutdown;
  elements.forceKillButton.disabled = !router?.can_force_kill;
  setHTML(elements.routerStatus, html`${[
    fact("Running", router?.running ? "yes" : "no"),
    fact("Managed", router?.managed ? "yes" : "no"),
    fact("URL", router?.url || "unknown"),
    fact("PID", router?.pid ? String(router.pid) : "none"),
    fact("Can shut down", router?.can_shutdown ? "yes" : "no"),
    fact("Can force kill", router?.can_force_kill ? "yes" : "no"),
    fact("Last error", router?.error || "none")
  ]}`);
}

function routerSummaryText(router: RouterProcessStatus | null): string {
  if (!router) {
    return "Router status unknown";
  }
  const pid = router.pid ? ` · pid ${router.pid}` : "";
  return `${router.url || "router"} · ${router.running ? "running" : "stopped"}${pid}`;
}

function routerTone(router: RouterProcessStatus | null): Tone {
  if (!router) {
    return "neutral";
  }
  if (router.error) {
    return "danger";
  }
  return router.running ? "success" : "warning";
}

export function renderTables(): void {
  renderModelInventory();
}

export function renderRecipes(): void {
  const recipes = state.inventory?.recipes ?? [];
  elements.recipeCount.textContent = `${recipes.length} recipe${recipes.length === 1 ? "" : "s"}`;
  setHTML(elements.recipesList, recipes.length > 0
    ? html`${recipes.map(renderRecipe)}`
    : html`<div class="card empty-state">No recipes yet. Combine models in Advanced cook to create one.</div>`);
}

function renderRecipe(recipe: Recipe): SafeHTML {
  const components = recipeLanes.flatMap(lane => recipe[lane] ? [recipe[lane]] : []);
  return html`
    <article class="card recipe-item" data-recipe-id="${recipe.id}">
      <header class="recipe-head">
        <div>
          <h3>${recipe.public_id || recipe.id}</h3>
          ${recipe.public_image_id ? html`<p class="muted">Image route ${recipe.public_image_id}</p>` : ""}
        </div>
        <button class="danger" type="button" data-delete-recipe="${recipe.id}">Delete</button>
      </header>
      <ul class="recipe-components">${components.map(renderRecipeComponent)}</ul>
    </article>
  `;
}

function renderRecipeComponent(component: RecipeComponent): SafeHTML {
  return html`
    <li>
      ${badge(component.kind, laneAccent(component.kind))}
      <code>${component.model_id || component.image_id || component.config_filename}</code>
      <span class="muted">${component.node_id}</span>
    </li>
  `;
}
