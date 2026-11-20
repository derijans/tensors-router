import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";


const {FakeControl, controls} = vi.hoisted(() => {
  class FakeControl {
    disabled = false;
    dataset: Record<string, string> = {};
    innerHTML = "";
    textContent = "";
    className = "";
    private readonly attributes = new Set<string>();

    toggleAttribute(name: string, force: boolean): boolean {
      if (force) {
        this.attributes.add(name);
      } else {
        this.attributes.delete(name);
      }
      return force;
    }

    hasAttribute(name: string): boolean {
      return this.attributes.has(name);
    }
  }
  return {FakeControl, controls: new Map<string, unknown>()};
});

vi.mock("../lib/dom", async importOriginal => ({
  ...await importOriginal<typeof import("../lib/dom")>(),
  getRequiredElement: (id: string): unknown => (new Proxy({}, {
    get: (_target, key: string) => {
      if (!controls.has(key)) {
        controls.set(key, new FakeControl());
      }
      return controls.get(key);
    }
  }) as Record<string, unknown>)[id]
}));

import { shellElements } from "../features/shell/shell-elements";
import { runOperation } from "../features/shell/operations";
import { renderRouterStatus } from "../features/shell/render-dashboard";
import { state } from "../state/app-state";

const routerButtons = () => [shellElements.launchButton, shellElements.restartButton, shellElements.shutdownButton, shellElements.forceKillButton];

describe("operation controls", () => {
  beforeEach(() => {
    for (const button of routerButtons()) {
      button.dataset.operationGroup = "router";
    }
    vi.stubGlobal("HTMLButtonElement", FakeControl);
    vi.stubGlobal("HTMLInputElement", FakeControl);
    vi.stubGlobal("HTMLSelectElement", FakeControl);
    vi.stubGlobal("document", {querySelectorAll: () => routerButtons()});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps a capability-blocked button disabled after a conflicting operation finishes", async () => {
    state.router = {managed: true, running: true, url: "https://127.0.0.1:8443", can_shutdown: false, can_force_kill: false};
    renderRouterStatus();

    await runOperation({key: "router-restart", group: "router", label: "Restarting router", task: () => Promise.resolve(undefined)});

    expect(shellElements.shutdownButton.disabled).toBe(true);
    expect(shellElements.forceKillButton.disabled).toBe(true);
    expect(shellElements.restartButton.disabled).toBe(false);
  });
});
