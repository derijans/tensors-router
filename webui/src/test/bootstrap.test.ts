import { describe, expect, it, vi } from "vitest";
import { bootstrapApplication } from "../bootstrap";

describe("bootstrapApplication", () => {
  it("opens the application after session recovery when initial data fails", async () => {
    const applySession = vi.fn();
    const showApp = vi.fn();
    const showLogin = vi.fn();
    await bootstrapApplication({
      getSession: () => Promise.resolve({authenticated: true, csrf: "csrf"}),
      applySession,
      showApp,
      showLogin,
      loadInitialData: () => Promise.reject(new Error("router unavailable"))
    });

    expect(applySession).toHaveBeenCalledWith({authenticated: true, csrf: "csrf"});
    expect(showApp).toHaveBeenCalledOnce();
    expect(showLogin).not.toHaveBeenCalled();
  });

  it("shows login only when session recovery fails", async () => {
    const applySession = vi.fn();
    const showApp = vi.fn();
    const showLogin = vi.fn();
    const loadInitialData = vi.fn();
    await bootstrapApplication({
      getSession: () => Promise.reject(new Error("unauthorized")),
      applySession,
      showApp,
      showLogin,
      loadInitialData
    });

    expect(applySession).not.toHaveBeenCalled();
    expect(showApp).not.toHaveBeenCalled();
    expect(loadInitialData).not.toHaveBeenCalled();
    expect(showLogin).toHaveBeenCalledOnce();
  });
});
