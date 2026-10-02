import { applyModelSearch } from "../bindings/model-bindings";
import { elements } from "../elements";
import { selectNode } from "../nodes-state";
import { SafeHTML, html, setHTML } from "../safe-html";
import { activateTab, navigableTabs, tabLabel } from "../shell/navigation";
import { state } from "../state";
import { showSelectedWebUIDialog } from "../webuis";
import { buildSearchIndex, searchEntries, searchKindLabels, type SearchEntry, type SearchTarget } from "./search-index";

let visibleResults: SearchEntry[] = [];

export function bindGlobalSearch(): void {
  const input = elements.globalSearchInput;
  input.addEventListener("input", () => {
    state.search.query = input.value;
    state.search.activeIndex = 0;
    renderResults();
  });
  input.addEventListener("focus", renderResults);
  input.addEventListener("keydown", handleSearchKey);
  input.closest(".search")?.addEventListener("focusout", event => {
    const next = (event as FocusEvent).relatedTarget;
    if (!(next instanceof Node) || !elements.globalSearchResults.contains(next)) {
      closeResults();
    }
  });
  elements.globalSearchResults.addEventListener("mousedown", event => event.preventDefault());
  elements.globalSearchResults.addEventListener("click", event => {
    const option = event.target instanceof Element ? event.target.closest<HTMLElement>("[data-search-index]") : null;
    const entry = option ? visibleResults[Number(option.dataset.searchIndex)] : undefined;
    if (entry) {
      openEntry(entry);
    }
  });
  document.addEventListener("keydown", event => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
      event.preventDefault();
      input.focus();
      input.select();
    }
  });
}

function handleSearchKey(event: KeyboardEvent): void {
  switch (event.key) {
    case "ArrowDown":
    case "ArrowUp":
      if (visibleResults.length > 0) {
        event.preventDefault();
        const step = event.key === "ArrowDown" ? 1 : -1;
        state.search.activeIndex = (state.search.activeIndex + step + visibleResults.length) % visibleResults.length;
        renderResults();
      }
      break;
    case "Enter": {
      const entry = visibleResults[state.search.activeIndex];
      if (entry) {
        event.preventDefault();
        openEntry(entry);
      }
      break;
    }
    case "Escape":
      closeResults();
      elements.globalSearchInput.blur();
      break;
  }
}

function renderResults(): void {
  const index = buildSearchIndex({
    sections: navigableTabs().map(tab => ({tab, label: tabLabel(tab)})),
    nodes: state.inventory?.nodes ?? [],
    models: state.inventory?.models?.length ? state.inventory.models : (state.inventory?.nodes ?? []).flatMap(node => node.models ?? []),
    recipes: state.inventory?.recipes ?? [],
    webuis: state.webuis.data?.data ?? []
  });
  visibleResults = searchEntries(index, state.search.query);
  const query = state.search.query.trim();
  const open = query !== "" && document.activeElement === elements.globalSearchInput;
  elements.globalSearchResults.hidden = !open;
  elements.globalSearchInput.setAttribute("aria-expanded", String(open));
  if (!open) {
    elements.globalSearchInput.removeAttribute("aria-activedescendant");
    return;
  }
  state.search.activeIndex = Math.min(state.search.activeIndex, Math.max(0, visibleResults.length - 1));
  setHTML(elements.globalSearchResults, visibleResults.length > 0 ? resultsMarkup() : html`<li class="search-empty">Nothing matches “${query}”.</li>`);
  if (visibleResults.length > 0) {
    elements.globalSearchInput.setAttribute("aria-activedescendant", optionID(state.search.activeIndex));
    document.getElementById(optionID(state.search.activeIndex))?.scrollIntoView({block: "nearest"});
  } else {
    elements.globalSearchInput.removeAttribute("aria-activedescendant");
  }
}

function resultsMarkup(): SafeHTML {
  return html`${visibleResults.map((entry, index) => {
    const startsGroup = index === 0 || visibleResults[index - 1]?.kind !== entry.kind;
    return html`
      ${startsGroup ? html`<li class="search-group" role="presentation">${searchKindLabels[entry.kind]}</li>` : ""}
      <li id="${optionID(index)}" class="search-result" role="option" aria-selected="${index === state.search.activeIndex}" data-search-index="${index}">
        <span>${entry.label}</span>
        <small>${entry.detail}</small>
      </li>
    `;
  })}`;
}

function optionID(index: number): string {
  return `globalSearchOption-${index}`;
}

function closeResults(): void {
  elements.globalSearchResults.hidden = true;
  elements.globalSearchInput.setAttribute("aria-expanded", "false");
  elements.globalSearchInput.removeAttribute("aria-activedescendant");
}

function openEntry(entry: SearchEntry): void {
  elements.globalSearchInput.value = "";
  state.search.query = "";
  closeResults();
  elements.globalSearchInput.blur();
  openTarget(entry.target);
}

function openTarget(target: SearchTarget): void {
  switch (target.kind) {
    case "section":
      activateTab(target.tab);
      break;
    case "node":
      activateTab("nodes");
      if (!state.nodes.expanded.includes(target.nodeID)) {
        selectNode(target.nodeID);
      }
      break;
    case "model":
      activateTab("models");
      applyModelSearch(target.modelID);
      break;
    case "recipe":
      activateTab("recipes");
      document.querySelector(`[data-recipe-id="${CSS.escape(target.recipeID)}"]`)?.scrollIntoView({block: "center"});
      break;
    case "webui":
      activateTab("webuis");
      showSelectedWebUIDialog(target.webuiID);
      break;
  }
}
