import { SafeHTML, html, setHTML } from "../../lib/safe-html";
import { state } from "../../state/app-state";
import { renderAnalytics } from "../insights/analytics/analytics";
import { renderConstructor } from "../cook/constructor/constructor-render";
import { renderBenchmarks } from "../insights/benchmarks/benchmarks";
import { renderSimpleCook } from "../cook/simple-cook/simple-cook-render";
import { renderModelInventory } from "../models/model-inventory";
import { renderNodesPanel } from "../nodes/nodes-render";
import { renderOverview } from "../overview/overview";
import { badge } from "../../ui/badge";
import { setControlUnavailable } from "./operations";
import type { Recipe, RecipeComponent, RouterProcessStatus, Tone } from "../../types";
import { pluralSuffix } from "../../lib/text";
import { laneAccent } from "../../domain/lanes";
import { fact } from "../../ui/fact-grid";
import { emptyCard } from "../../ui/empty-state";
import { mutedText } from "../../ui/text";
import { cookElements } from "../cook/cook-elements";
import { shellElements } from "./shell-elements";

const recipeLanes = ["text", "image", "embeddings", "voice", "music"] as const;

export function showLogin(): void {
  shellElements.loginView.classList.remove("hidden");
  shellElements.appView.classList.add("hidden");
}

export function showApp(): void {
  shellElements.loginView.classList.add("hidden");
  shellElements.appView.classList.remove("hidden");
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
  shellElements.routerSummary.textContent = routerSummaryText(router);
  shellElements.routerSummary.className = `router-chip tone-${routerTone(router)}`;
  setControlUnavailable(shellElements.launchButton, !router?.managed || Boolean(router?.running));
  setControlUnavailable(shellElements.restartButton, !router?.managed);
  setControlUnavailable(shellElements.shutdownButton, !router?.can_shutdown);
  setControlUnavailable(shellElements.forceKillButton, !router?.can_force_kill);
  setHTML(shellElements.routerStatus, html`${[
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
  shellElements.recipeCount.textContent = `${recipes.length} recipe${pluralSuffix(recipes.length)}`;
  setHTML(cookElements.recipesList, recipes.length > 0
    ? html`${recipes.map(renderRecipe)}`
    : emptyCard("No recipes yet. Combine models in Advanced cook to create one."));
}

function renderRecipe(recipe: Recipe): SafeHTML {
  const components = recipeLanes.flatMap(lane => recipe[lane] ? [recipe[lane]] : []);
  return html`
    <article class="card recipe-item" data-recipe-id="${recipe.id}">
      <header class="recipe-head">
        <div>
          <h3>${recipe.public_id || recipe.id}</h3>
          ${recipe.public_image_id ? mutedText(html`Image route ${recipe.public_image_id}`, "p") : ""}
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
      ${mutedText(component.node_id)}
    </li>
  `;
}
