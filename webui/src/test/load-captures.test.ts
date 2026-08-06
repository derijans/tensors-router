import { describe, expect, it } from "vitest";
import { stripTerminalControls } from "../terminal-output";

describe("load capture display", () => {
  it("removes terminal effects and unsafe control bytes while retaining text", () => {
    const value = "plain\u001b[31mred\u001b[0m\u0000\u0007tail\n";
    expect(stripTerminalControls(value)).toBe("plainredtail\n");
  });

  it("removes OSC title sequences", () => {
    expect(stripTerminalControls("before\u001b]0;secret title\u0007after")).toBe("beforeafter");
  });
});