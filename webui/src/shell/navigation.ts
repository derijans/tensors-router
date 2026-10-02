import { queryElements } from "../dom";
import { elements } from "../elements";
import { state } from "../state";

type TabActivationListener = (activeTab: string) => void;

const activationListeners: TabActivationListener[] = [];

export function onTabActivation(listener: TabActivationListener): void {
  activationListeners.push(listener);
}

export function activateTab(name: string): void {
  const tabButton = tabButtonFor(name);
  if (!tabButton || tabButton.hidden) {
    return;
  }
  state.activeTab = name;
  queryElements("[data-tab]", HTMLButtonElement).forEach(button => {
    const active = button.dataset.tab === name;
    button.classList.toggle("active", active);
    button.toggleAttribute("aria-current", active);
  });
  queryElements("[data-panel]", HTMLElement).forEach(panel => panel.classList.toggle("active", panel.dataset.panel === name));
  elements.shellSection.textContent = tabButton.textContent?.trim() ?? "";
  activationListeners.forEach(listener => listener(name));
}

export function tabLabel(name: string): string {
  return tabButtonFor(name)?.textContent?.trim() ?? name;
}

export function navigableTabs(): string[] {
  return queryElements("[data-tab]", HTMLButtonElement)
    .filter(button => !button.hidden)
    .map(button => button.dataset.tab ?? "")
    .filter(Boolean);
}

export function bindNavigation(): void {
  queryElements("[data-tab]", HTMLButtonElement).forEach(button => {
    button.addEventListener("click", () => activateTab(button.dataset.tab ?? ""));
  });
  document.addEventListener("click", event => {
    const target = event.target instanceof Element ? event.target.closest<HTMLElement>("[data-navigate]") : null;
    if (target?.dataset.navigate) {
      activateTab(target.dataset.navigate);
    }
  });
}

function tabButtonFor(name: string): HTMLButtonElement | undefined {
  return queryElements("[data-tab]", HTMLButtonElement).find(button => button.dataset.tab === name);
}
